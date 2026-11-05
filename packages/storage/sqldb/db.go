/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/migration"
	"github.com/IBAX-io/go-ibax/packages/storage/kvdb/redis"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	// DBConn is orm connection
	DBConn *gorm.DB

	// ErrRecordNotFound is Not Found Record wrapper
	ErrRecordNotFound = gorm.ErrRecordNotFound

	// ErrDBConn database connection error
	ErrDBConn = errors.New("database connection error")
)
var notAutoIncrement = map[string]bool{
	"1_keys": true,
}

// non-self-increasing costs
const notAutoIncrementCost int64 = 1

type KeyTableChecker struct{}

func (ktc KeyTableChecker) IsKeyTable(tableName string) bool {
	val, exist := converter.FirstEcosystemTables[tableName]
	return exist && val
}

type NextIDGetter struct {
	Tx *DbTransaction
}

func (g NextIDGetter) GetNextID(tableName string) (int64, error) {
	return g.Tx.GetNextID(tableName)
}
func isFound(db *gorm.DB) (bool, error) {
	if errors.Is(db.Error, ErrRecordNotFound) {
		return false, nil
	}
	return true, db.Error
}

// InitDB drop all tables and exec db schema
func InitDB(cfg conf.DBConfig) error {
	err := GormInit(cfg)
	if err != nil || DBConn == nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("initializing DB")
		return ErrDBConn
	}
	if err = NewDbTransaction(DBConn).DropTables(); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("dropping all tables")
		return err
	}

	if conf.Config.Redis.Enable {
		err = redis.RedisInit(conf.Config.Redis)
		if err != nil {
			log.WithFields(log.Fields{
				"host": conf.Config.Redis.Host, "port": conf.Config.Redis.Port, "db_password": conf.Config.Redis.Password, "db_name": conf.Config.Redis.DbName, "type": consts.DBError,
			}).Error("can't init redis")
			return err
		}

		var rd redis.RedisParams
		err := rd.Cleardb()
		if err != nil {
			return err
		}

	}

	if err = ExecSchema(); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("executing db schema")
		return err
	}

	install := &Install{Progress: ProgressComplete}
	if err = install.Create(); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating install")
		return err
	}

	if err := ExecChildChainSchema(consts.DefaultCLB, conf.Config.KeyID); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating CLB schema")
		return err
	}
	if err := ExecSubSchema(); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating CLB schema")
		return err
	}

	return nil
}

func dsnOf(cfg conf.DBConfig) string {
	return fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=disable password=%s TimeZone=UTC", cfg.Host, cfg.Port, cfg.User, cfg.Name, cfg.Password)
}

// GormInit is initializes Gorm connection: DBConn, the pool of the API and the daemons, and
// the pool the blocks run in, so a block never waits for a connection a request holds
func GormInit(cfg conf.DBConfig) error {
	dsn := dsnOf(cfg)
	// every connection takes the settings of the node as it opens, so no transaction sets them
	params := map[string]string{
		"lock_timeout":                        strconv.Itoa(cfg.LockTimeout),
		"idle_in_transaction_session_timeout": strconv.Itoa(cfg.IdleInTxTimeout),
	}
	db, err := openGorm(dsn, params)
	if err != nil && strings.Contains(err.Error(), "SQLSTATE 3D000") {
		err = createDatabase(fmt.Sprintf("host=%s port=%d user=%s password=%s sslmode=disable TimeZone=UTC", cfg.Host, cfg.Port, cfg.User, cfg.Password), cfg.Name)
		if err == nil {
			db, err = openGorm(dsn, params)
		}
	}
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("cant open connection to DB")
		return err
	}
	exec, err := openGorm(dsn, params)
	if err != nil {
		closeGorm(db)
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("cant open connection to DB")
		return err
	}
	setPool(db, cfg.MaxIdleConns, cfg.MaxOpenConns)
	n := execConns(conf.Config.Exec.WorkerCount())
	setPool(exec, n, n)
	DBConn, execConn = db, exec
	return nil
}

// execConn is the pool of the blocks: the transaction of a block, the connections its workers
// hold, and the one the block runs transactions again on
var execConn *gorm.DB

// execConns is the size of the pool of the blocks whose workers are workers: a connection each,
// one for the transaction of the block, one it runs transactions again on, and two for the
// rollback and the BFT round a block may overlap
func execConns(workers int) int { return workers + 4 }

// execDB is the pool the blocks run in; DBConn when GormInit opened none
func execDB() *gorm.DB {
	if execConn != nil {
		return execConn
	}
	return DBConn
}

// openGorm opens a pool of pgx connections with the statement count and the transaction cache
func openGorm(dsn string, params map[string]string) (*gorm.DB, error) {
	sqlDB, err := openDB(dsn, params)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		// The extended protocol: pgx prepares a statement once on a connection and keeps it in
		// its statement cache, so a statement a block repeats is planned once
		Conn: sqlDB,
	}), &gorm.Config{
		AllowGlobalUpdate: true,                                  //allow global update
		Logger:            logger.Default.LogMode(logger.Silent), // start Logger, show detail log
	})
	if err != nil {
		sqlDB.Close()
		return nil, err
	}
	if err = registerStatementCount(db); err != nil {
		sqlDB.Close()
		return nil, err
	}
	if err = registerTxCache(db); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func setPool(db *gorm.DB, idle, open int) {
	sqlDB, _ := db.DB()
	sqlDB.SetConnMaxLifetime(time.Minute * 10)
	sqlDB.SetMaxIdleConns(idle)
	sqlDB.SetMaxOpenConns(open)
}

func closeGorm(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func createDatabase(dsn string, dbName string) error {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	if err != nil {
		return err
	}
	result := db.Exec("create database " + dbName)
	defer func() {
		d, _ := db.DB()
		d.Close()
	}()
	return result.Error
}

// GormClose is closing Gorm connection
func GormClose() error {
	var err error
	if execConn != nil {
		err = closeGorm(execConn)
		execConn = nil
	}
	if DBConn != nil {
		err = errors.Join(err, closeGorm(DBConn))
		DBConn = nil
	}
	return err
}

// DbTransaction is gorm.DB wrapper
type DbTransaction struct {
	conn       *gorm.DB
	statements *atomic.Int64
	cache      *txCache
	// view is the view on the state of the block the statements of a transaction run in
	view *TxView
}

// View is the view on the state of the block the transaction runs in; nil outside one
func (tr *DbTransaction) View() *TxView {
	if tr == nil {
		return nil
	}
	return tr.view
}

// ForgetCache empties the cache of the transaction
func (tr *DbTransaction) ForgetCache() {
	if tr != nil && tr.cache != nil {
		tr.cache.forgetAll()
	}
}

func NewDbTransaction(conn *gorm.DB) *DbTransaction {
	return &DbTransaction{conn: conn}
}

func (d *DbTransaction) Debug() *DbTransaction {
	d.conn = d.conn.Debug()
	return d
}

// StartTransaction is beginning transaction, in the pool of the blocks
func StartTransaction() (*DbTransaction, error) {
	ctx, statements := withStatementCount()
	ctx, cache := withTxCache(ctx)
	conn := execDB().WithContext(ctx).Begin()
	if conn.Error != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": conn.Error}).Error("cannot start transaction because of connection error")
		return nil, conn.Error
	}

	return &DbTransaction{
		conn:       conn,
		statements: statements,
		cache:      cache,
	}, nil
}

// Rollback is transaction rollback
func (tr *DbTransaction) Rollback() error {
	return tr.conn.Rollback().Error
}

// Commit is transaction commit; a transaction StartTransaction began counts in Commits
func (tr *DbTransaction) Commit() error {
	if err := tr.conn.Commit().Error; err != nil {
		return err
	}
	if tr.statements != nil {
		commits.Add(1)
	}
	return nil
}

// Connection returns connection of database
func (tr *DbTransaction) Connection() *gorm.DB {
	return tr.conn
}

// Savepoint creates PostgreSQL Savepoint. It runs the statement itself: the savepoints of gorm
// drop its error.
func (tr *DbTransaction) Savepoint(mark string) error {
	return tr.Connection().Exec("SAVEPOINT " + mark).Error
}

// RollbackSavepoint rollbacks PostgreSQL Savepoint, with the error of the statement
func (tr *DbTransaction) RollbackSavepoint(mark string) error {
	return tr.Connection().Exec("ROLLBACK TO SAVEPOINT " + mark).Error
}

// ReleaseSavepoint releases PostgreSQL Savepoint
func (tr *DbTransaction) ReleaseSavepoint(mark string) error {
	return tr.Connection().Exec("RELEASE SAVEPOINT " + mark).Error
}

func (tr *DbTransaction) ResetSavepoint(mark string) error {
	if err := tr.RollbackSavepoint(mark); err != nil {
		return err
	}
	return tr.Savepoint(mark)
}

// GetDB is returning gorm.DB
func GetDB(tr *DbTransaction) *gorm.DB {
	if tr != nil && tr.conn != nil {
		return tr.conn
	}
	return DBConn
}

// DropTables is dropping all of the tables, with the schemas of the partitions
func (dbTx *DbTransaction) DropTables() error {
	for _, s := range []string{migration.PartsSchema, migration.RetiredSchema, migration.ArchiveSchema} {
		if err := GetDB(dbTx).Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, s)).Error; err != nil {
			return err
		}
	}
	return GetDB(dbTx).Exec(`
	DO $$ DECLARE
	    r RECORD;
	BEGIN
	    FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname = current_schema()) LOOP
		EXECUTE 'DROP TABLE IF EXISTS ' || quote_ident(r.tablename) || ' CASCADE';
	    END LOOP;
	END $$;
	`).Error
}

// GetRecordsCountTx is counting all records of table in transaction
func (dbTx *DbTransaction) GetRecordsCountTx(tableName, where string) (count int64, err error) {
	if !notAutoIncrement[tableName] {
		if len(where) == 0 && !strings.ContainsAny(tableName, `" `) {
			return dbTx.maxID(tableName)
		}
		dbQuery := GetDB(dbTx).Table(tableName)
		if len(where) > 0 {
			dbQuery = dbQuery.Where(where)
		}
		err := dbQuery.Select("id").Order("id DESC").Limit(1).Scan(&count).Error
		if err != nil {
			return 0, err
		}
	} else {
		//err = dbQuery.Count(&count).Error
		count = notAutoIncrementCost
	}
	return count, err
}

// Update is updating table rows. args are the values bound to the placeholders of set and where.
func (dbTx *DbTransaction) Update(tblname, set, where string, args ...any) error {
	sql := `UPDATE "` + strings.Trim(tblname, `"`) + `" SET ` + set + " " + where
	return dbTx.ExecSql(sql, args...)
}

// ExecSql executes the statement with args bound to its placeholders
func (dbTx *DbTransaction) ExecSql(sql string, args ...any) error {
	return dbTx.exec(sql, args...)
}

// Delete is deleting table rows. args are the values bound to the placeholders of where.
func (dbTx *DbTransaction) Delete(tblname, where string, args ...any) error {
	return dbTx.ExecSql(`DELETE FROM "`+tblname+`" `+where, args...)
}

// GetColumnCount is counting rows in table
func (dbTx *DbTransaction) GetColumnCount(tableName string) (int64, error) {
	var count int64
	err := GetDB(dbTx).Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name=?", tableName).Row().Scan(&count)
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("executing raw query")
		return 0, err
	}
	return count, nil
}

// AlterTableAddColumn is adding column to table
func (dbTx *DbTransaction) AlterTableAddColumn(tableName, columnName, columnType string) error {
	return dbTx.ExecSql(`ALTER TABLE "` + tableName + `" ADD COLUMN "` + columnName + `" ` + columnType)
}

// AlterTableDropColumn is dropping column from table
func (dbTx *DbTransaction) AlterTableDropColumn(tableName, columnName string) error {
	return dbTx.ExecSql(`ALTER TABLE "` + tableName + `" DROP COLUMN "` + columnName + `"`)
}

// CreateIndex is creating index on table column
func (dbTx *DbTransaction) CreateIndex(indexName, tableName, onColumn string) error {
	return GetDB(dbTx).Exec(`CREATE INDEX "` + indexName + `_index" ON "` + tableName + `" (` + onColumn + `)`).Error
}

// GetColumnDataTypeCharMaxLength is returns max length of table column
func (dbTx *DbTransaction) GetColumnDataTypeCharMaxLength(tableName, columnName string) (map[string]string, error) {
	return dbTx.GetOneRow(`select data_type,character_maximum_length from
			 information_schema.columns where table_schema = current_schema() AND table_name = ? AND column_name = ?`,
		tableName, columnName).String()
}

// GetAllColumnTypes returns column types for table
func (dbTx *DbTransaction) GetAllColumnTypes(tblname string) ([]map[string]string, error) {
	return dbTx.GetAllTransaction(`SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ?
		ORDER BY ordinal_position ASC`, -1, tblname)
}

func DataTypeToColumnType(dataType string) string {
	var itype string
	switch {
	case dataType == "character varying":
		itype = `varchar`
	case dataType == `bigint`:
		itype = "number"
	case dataType == `jsonb`:
		itype = "json"
	case strings.HasPrefix(dataType, `timestamp`):
		itype = "datetime"
	case strings.HasPrefix(dataType, `numeric`):
		itype = "money"
	case strings.HasPrefix(dataType, `double`):
		itype = "double"
	case strings.HasPrefix(dataType, `bytea`):
		itype = "bytea"
	default:
		itype = dataType
	}
	return itype
}

// GetColumnType is returns type of column
func (dbTx *DbTransaction) GetColumnType(tblname, column string) (itype string, err error) {
	coltype, err := dbTx.GetColumnDataTypeCharMaxLength(tblname, column)
	if err != nil {
		return
	}
	if dataType, ok := coltype["data_type"]; ok {
		itype = DataTypeToColumnType(dataType)
	}
	return
}

// DropTable is dropping table
func (dbTx *DbTransaction) DropTable(tableName string) error {
	return GetDB(dbTx).Migrator().DropTable(tableName)
}

// NumIndexes is counting table indexes
func (dbTx *DbTransaction) NumIndexes(tblname string) (int, error) {
	var indexes int64
	return cached(dbTx, schemaTable, "indexes:"+tblname, func() (int, error) {
		err := GetDB(dbTx).Raw(fmt.Sprintf(`select count( i.relname) from pg_class t, pg_class i, pg_index ix, pg_attribute a
	 where t.oid = ix.indrelid and i.oid = ix.indexrelid and a.attrelid = t.oid and a.attnum = ANY(ix.indkey)
         and t.relkind = 'r' and t.relnamespace = current_schema()::regnamespace and t.relname = '%s'`, tblname)).Row().Scan(&indexes)
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		return int(indexes - 1), nil
	})
}

// IsIndex returns is table column is an index
func (dbTx *DbTransaction) IsIndex(tblname, column string) (bool, error) {
	row, err := dbTx.GetOneRow(`select t.relname as table_name, i.relname as index_name, a.attname as column_name
	 from pg_class t, pg_class i, pg_index ix, pg_attribute a 
	 where t.oid = ix.indrelid and i.oid = ix.indexrelid and a.attrelid = t.oid and a.attnum = ANY(ix.indkey)
		 and t.relkind = 'r' and t.relnamespace = current_schema()::regnamespace and t.relname = ?  and a.attname = ?`, tblname, column).String()
	return len(row) > 0 && row[`column_name`] == column, err
}

// GetNextID returns next ID of table
func (dbTx *DbTransaction) GetNextID(table string) (int64, error) {
	id, err := dbTx.maxID(table)
	if err != nil {
		return 0, err
	}
	if t, ok := migration.PartitionedTableOf(table); ok && t.ID {
		floor, err := dbTx.idFloor(table)
		if err != nil {
			return 0, err
		}
		id = max(id, floor)
	}
	return id + 1, nil
}

// IsTable returns is table exists
func (dbTx *DbTransaction) IsTable(tblname string) bool {
	var name string
	err := GetDB(dbTx).Table("information_schema.tables").
		Where("table_type = 'BASE TABLE' AND table_schema = current_schema() AND table_name=?", tblname).
		Select("table_name").Row().Scan(&name)
	if err != nil {
		return false
	}

	return name == tblname
}

func (dbTx *DbTransaction) HasTableOrView(names string) bool {
	var name string
	GetDB(dbTx).Table("information_schema.tables").
		Where("table_type IN ('BASE TABLE', 'VIEW') AND table_schema = current_schema() AND table_name=?", names).
		Select("table_name").Row().Scan(&name)

	return name == names
}

// GetColumnByID returns the value of the column from the table by id
func GetColumnByID(table, column string, id int64) (result string, err error) {
	dbTr := new(DbTransaction)
	columnList, err := dbTr.GetAllColumnTypes(table)
	if err != nil {
		return "", err
	}
	var exist bool
	for _, list := range columnList {
		for k, v := range list {
			if k == "column_name" {
				if v == column {
					exist = true
					break
				}
			}
		}
		if exist {
			break
		}
	}
	if !exist {
		return "", errors.New("column invalid")
	}

	err = GetDB(nil).Table(table).Select(column).Where("id=?", id).Row().Scan(&result)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting column by id")
	}
	return
}

// DropDatabase kill all process and drop database
func (dbTx *DbTransaction) DropDatabase(name string) error {
	query := `SELECT
	pg_terminate_backend (pg_stat_activity.pid)
   FROM
	pg_stat_activity
   WHERE
	pg_stat_activity.datname = ?`

	if err := GetDB(dbTx).Exec(query, name).Error; err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err, "dbname": name}).Error("on kill db process")
		return err
	}

	if err := GetDB(dbTx).Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", name)).Error; err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err, "dbname": name}).Error("on drop db")
		return err
	}

	return nil
}

// GetSumColumn returns the value of the column from the table by id
func (dbTx *DbTransaction) GetSumColumn(table, column, where string) (result string, err error) {
	err = GetDB(dbTx).Table(table).Select("sum(" + column + ")").Where(where).Row().Scan(&result)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("sum column")
	}
	return
}

// GetSumColumnCount returns the value of the column from the table by id
func (dbTx *DbTransaction) GetSumColumnCount(table, column, where string) (result int, err error) {
	err = GetDB(dbTx).Table(table).Select("count(*)").Where(where).Row().Scan(&result)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("sum column")
	}
	return
}

type Namer struct {
	TableType string
}

type SchemaInter interface {
	HasExists(tr *DbTransaction, name string) bool
}

func (v Namer) HasExists(tr *DbTransaction, names string) bool {
	var typs string
	switch v.TableType {
	case "table":
		typs = `= 'BASE TABLE'`
	case "view":
		typs = `= 'VIEW'`
	default:
		typs = `IN ('BASE TABLE', 'VIEW')`
	}
	var name string
	GetDB(tr).Table("information_schema.tables").
		Where(fmt.Sprintf("table_type %s AND table_schema = current_schema() AND table_name='%s'", typs, names)).
		Select("table_name").Row().Scan(&name)
	return name == names
}
