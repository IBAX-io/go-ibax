/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/migration"
	"gorm.io/gorm"
)

// The retention of rows is a rule of the chain: every node executing a block of a height multiple
// of migration.PartitionBlocks (a boundary) takes out of the active tables, in the transaction of
// the block, the same partitions, so that what the contracts read is the same on every node.
//
// A partition taken out at a boundary waits in the retired schema until the next boundary, where a
// validator drops it and an archive node attaches it to the table of its name in the archive
// schema: the rollback of the boundary block puts back the partitions it took out, so that the
// blocks before it execute again on the rows they read the first time.

// rollbackFloorName names, in retention_state, the lowest block the chain can be rolled back to:
// undoing a block at or below it needs rows the retention has dropped
const rollbackFloorName = "rollback_floor"

// ErrBeyondRetention is a rollback to a block whose rows are no longer kept
var ErrBeyondRetention = errors.New("the rows to roll back to the block are no longer kept")

// IsRetentionBoundary is whether the retention runs at the block
func IsRetentionBoundary(blockID int64) bool {
	return blockID > 0 && blockID%migration.PartitionBlocks == 0
}

// Retain runs the retention at the boundary block blockID of time blockTime (seconds), with the
// window of historyBlocks blocks
func Retain(dbTx *DbTransaction, blockID, blockTime, historyBlocks int64) error {
	db := GetDB(dbTx)
	k := blockID / migration.PartitionBlocks
	for _, t := range migration.PartitionedTables {
		if err := db.Exec(migration.CreatePartitionSQL(t.Name, k+1)).Error; err != nil {
			return fmt.Errorf("creating the partition %d of %s: %w", k+1, t.Name, err)
		}
	}
	// the transactions of range k-1 are at most as late as its last block, blockID-1
	if err := db.Exec(`INSERT INTO "log_transaction_ranges" ("k", "last_time")
		SELECT ?, "time" FROM "block_chain" WHERE "id" = ?
		ON CONFLICT ("k") DO UPDATE SET "last_time" = EXCLUDED."last_time"`, k-1, blockID-1).Error; err != nil {
		return fmt.Errorf("recording the time of range %d: %w", k-1, err)
	}
	if err := finishRetired(dbTx, blockID); err != nil {
		return err
	}
	for _, t := range migration.PartitionedTables {
		ranges, err := attachedRanges(dbTx, t.Name)
		if err != nil {
			return err
		}
		for _, r := range ranges {
			out, err := outOfWindow(dbTx, t, r, blockID, blockTime, historyBlocks)
			if err != nil {
				return err
			}
			if !out {
				break
			}
			if err := retire(dbTx, t, r, blockID); err != nil {
				return err
			}
		}
	}
	// a spent output leaves with the block that spent it, to the archive schema on an archive node;
	// an unspent one stays
	cut := blockID - historyBlocks
	del := `DELETE FROM "spent_info" WHERE "spent_block" BETWEEN 1 AND ?`
	if conf.Config.Storage.Archive {
		del = fmt.Sprintf(`WITH "spent" AS (%s RETURNING *) INSERT INTO "%s"."spent_info" SELECT * FROM "spent"`,
			del, migration.ArchiveSchema)
	}
	res := db.Exec(del, cut-1)
	if res.Error != nil {
		return fmt.Errorf("deleting the spent outputs: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		return raiseRollbackFloor(dbTx, cut-1)
	}
	return nil
}

// outOfWindow is whether the range r of the table t is out of the window at the boundary blockID
func outOfWindow(dbTx *DbTransaction, t migration.PartitionedTable, r, blockID, blockTime, historyBlocks int64) (bool, error) {
	end := (r + 1) * migration.PartitionBlocks
	if !t.ByTime {
		return end <= blockID-historyBlocks, nil
	}
	if end > blockID {
		return false, nil
	}
	// a transaction older than MaxTxBack is refused anyway: the range of transactions all older
	// than that leaves without letting one in twice
	var last []int64
	if err := GetDB(dbTx).Raw(`SELECT "last_time" FROM "log_transaction_ranges" WHERE "k" = ?`, r).
		Scan(&last).Error; err != nil {
		return false, err
	}
	return len(last) == 1 && last[0] < blockTime-consts.MaxTxBack, nil
}

// attachedRanges are, in order, the ranges of the partitions of the table in the active table
func attachedRanges(dbTx *DbTransaction, table string) ([]int64, error) {
	var names []string
	if err := GetDB(dbTx).Raw(`SELECT c.relname FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE i.inhparent = ?::regclass AND n.nspname = ?`, `"`+table+`"`, migration.PartsSchema).
		Scan(&names).Error; err != nil {
		return nil, fmt.Errorf("listing the partitions of %s: %w", table, err)
	}
	prefix := table + "_p"
	ranges := make([]int64, 0, len(names))
	for _, name := range names {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		r, err := strconv.ParseInt(name[len(prefix):], 10, 64)
		if err != nil {
			continue
		}
		ranges = append(ranges, r)
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i] < ranges[j] })
	return ranges, nil
}

// retire takes the range r of the table t out of the active table at the boundary blockID
func retire(dbTx *DbTransaction, t migration.PartitionedTable, r, blockID int64) error {
	db := GetDB(dbTx)
	part := migration.PartitionName(t.Name, r)
	for _, sql := range []string{
		fmt.Sprintf(`ALTER TABLE "%s" DETACH PARTITION "%s"."%s"`, t.Name, migration.PartsSchema, part),
		fmt.Sprintf(`ALTER TABLE "%s"."%s" SET SCHEMA "%s"`, migration.PartsSchema, part, migration.RetiredSchema),
	} {
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("retiring %s: %w", part, err)
		}
	}
	if err := db.Exec(`INSERT INTO "retired_partitions" ("boundary", "table_name", "k") VALUES (?, ?, ?)`,
		blockID, t.Name, r).Error; err != nil {
		return err
	}
	if !t.ID {
		return nil
	}
	// the ids of the rows that left are not given again
	return db.Exec(fmt.Sprintf(`INSERT INTO "partition_id_floors" ("table_name", "max_id")
		SELECT ?, coalesce(max("id"), 0) FROM "%s"."%s"
		ON CONFLICT ("table_name") DO UPDATE
		SET "max_id" = GREATEST("partition_id_floors"."max_id", EXCLUDED."max_id")`, migration.RetiredSchema, part),
		t.Name).Error
}

type retiredPartition struct {
	Boundary  int64
	TableName string
	K         int64
}

func retiredBefore(dbTx *DbTransaction, cond string, blockID int64) ([]retiredPartition, error) {
	var rs []retiredPartition
	err := GetDB(dbTx).Raw(`SELECT "boundary", "table_name", "k" FROM "retired_partitions"
		WHERE "boundary" `+cond+` ? ORDER BY "boundary", "table_name", "k"`, blockID).Scan(&rs).Error
	return rs, err
}

// finishRetired drops, or on an archive node archives, the partitions retired before the boundary
// blockID
func finishRetired(dbTx *DbTransaction, blockID int64) error {
	rs, err := retiredBefore(dbTx, "<", blockID)
	if err != nil || len(rs) == 0 {
		return err
	}
	db := GetDB(dbTx)
	var floor int64
	for _, r := range rs {
		part := migration.PartitionName(r.TableName, r.K)
		if conf.Config.Storage.Archive {
			err = archivePartition(dbTx, r.TableName, r.K)
		} else {
			err = db.Exec(fmt.Sprintf(`DROP TABLE "%s"."%s"`, migration.RetiredSchema, part)).Error
		}
		if err != nil {
			return fmt.Errorf("finishing %s: %w", part, err)
		}
		floor = max(floor, r.Boundary)
	}
	if err := db.Exec(`DELETE FROM "retired_partitions" WHERE "boundary" < ?`, blockID).Error; err != nil {
		return err
	}
	return raiseRollbackFloor(dbTx, floor)
}

// RestoreRetired puts back in the active tables the partitions the boundary block blockID took
// out, as it is rolled back
func RestoreRetired(dbTx *DbTransaction, blockID int64) error {
	rs, err := retiredBefore(dbTx, "=", blockID)
	if err != nil || len(rs) == 0 {
		return err
	}
	db := GetDB(dbTx)
	for _, r := range rs {
		part := migration.PartitionName(r.TableName, r.K)
		for _, sql := range []string{
			fmt.Sprintf(`ALTER TABLE "%s"."%s" SET SCHEMA "%s"`, migration.RetiredSchema, part, migration.PartsSchema),
			fmt.Sprintf(`ALTER TABLE "%s" ATTACH PARTITION "%s"."%s" FOR VALUES %s`, r.TableName,
				migration.PartsSchema, part, migration.PartitionBounds(r.TableName, r.K)),
		} {
			if err := db.Exec(sql).Error; err != nil {
				return fmt.Errorf("restoring %s: %w", part, err)
			}
		}
	}
	return db.Exec(`DELETE FROM "retired_partitions" WHERE "boundary" = ?`, blockID).Error
}

func raiseRollbackFloor(dbTx *DbTransaction, floor int64) error {
	return GetDB(dbTx).Exec(`INSERT INTO "retention_state" ("name", "value") VALUES (?, ?)
		ON CONFLICT ("name") DO UPDATE SET "value" = GREATEST("retention_state"."value", EXCLUDED."value")`,
		rollbackFloorName, floor).Error
}

// RollbackFloor is the lowest block the chain can be rolled back to
func RollbackFloor(dbTx *DbTransaction) (int64, error) {
	var v []int64
	if err := GetDB(dbTx).Raw(`SELECT "value" FROM "retention_state" WHERE "name" = ?`, rollbackFloorName).
		Scan(&v).Error; err != nil {
		return 0, err
	}
	if len(v) == 0 {
		return 0, nil
	}
	return v[0], nil
}

// CheckRollbackTo refuses a rollback that leaves blockID the last block when the rows to undo the
// blocks after it are no longer kept
func CheckRollbackTo(dbTx *DbTransaction, blockID int64) error {
	floor, err := RollbackFloor(dbTx)
	if err != nil {
		return err
	}
	if blockID < floor {
		return fmt.Errorf("%w: block %d, lowest %d", ErrBeyondRetention, blockID, floor)
	}
	return nil
}

// idFloorKey keys, in the cache of a transaction, the highest id of the rows of a table that left
const idFloorKey = "idfloor"

// idFloor is the highest id of the rows of a partitioned table that left the active table
func (tr *DbTransaction) idFloor(table string) (int64, error) {
	return cached(tr, table, idFloorKey, func() (int64, error) {
		var v []int64
		if err := GetDB(tr).Raw(`SELECT "max_id" FROM "partition_id_floors" WHERE "table_name" = ?`, table).
			Scan(&v).Error; err != nil {
			return 0, err
		}
		if len(v) == 0 {
			return 0, nil
		}
		return v[0], nil
	})
}

// archivedSpentInfo is the table of the archive schema of the spent outputs out of the window
var archivedSpentInfo = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%[1]s"."spent_info" (LIKE "spent_info" INCLUDING DEFAULTS INCLUDING INDEXES);
	CREATE OR REPLACE VIEW "%[1]s"."spent_info_all" AS SELECT * FROM "spent_info" UNION ALL SELECT * FROM "%[1]s"."spent_info";`,
	migration.ArchiveSchema)

// archiveTable is the table of the archive schema of the partitions out of the window of a table
func archiveTable(t migration.PartitionedTable) string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%[1]s"."%[2]s" (LIKE "%[2]s" INCLUDING DEFAULTS) PARTITION BY RANGE ("%[3]s");
		CREATE OR REPLACE VIEW "%[1]s"."%[2]s_all" AS SELECT * FROM "%[2]s" UNION ALL SELECT * FROM "%[1]s"."%[2]s";`,
		migration.ArchiveSchema, t.Name, t.Key)
}

// archivePartition attaches the retired range k of the table to its table in the archive schema
func archivePartition(dbTx *DbTransaction, table string, k int64) error {
	t, _ := migration.PartitionedTableOf(table)
	part := migration.PartitionName(table, k)
	db := GetDB(dbTx)
	for _, sql := range []string{
		archiveTable(t),
		fmt.Sprintf(`ALTER TABLE "%s"."%s" SET SCHEMA "%s"`, migration.RetiredSchema, part, migration.ArchiveSchema),
		fmt.Sprintf(`ALTER TABLE "%[1]s"."%[2]s" ATTACH PARTITION "%[1]s"."%[3]s" FOR VALUES %[4]s`,
			migration.ArchiveSchema, table, part, migration.PartitionBounds(table, k)),
	} {
		if err := db.Exec(sql).Error; err != nil {
			return err
		}
	}
	return nil
}

// PrepareArchive makes, on an archive node, the tables of the archive schema and the views that
// read a table with its archived rows
func PrepareArchive() error {
	if !conf.Config.Storage.Archive {
		return nil
	}
	for _, t := range migration.PartitionedTables {
		var exists bool
		if err := DBConn.Raw(`SELECT to_regclass(?) IS NOT NULL`, `"`+t.Name+`"`).Scan(&exists).Error; err != nil {
			return err
		}
		if !exists {
			// 1_history comes with the first block, and its archive with its first partition out
			continue
		}
		if err := DBConn.Exec(archiveTable(t)).Error; err != nil {
			return fmt.Errorf("preparing the archive of %s: %w", t.Name, err)
		}
	}
	if err := DBConn.Exec(archivedSpentInfo).Error; err != nil {
		return fmt.Errorf("preparing the archive of spent_info: %w", err)
	}
	return nil
}

// archived reads the rows of the table wherever they are, for the API and the synchronization
func archived(table string) *gorm.DB {
	return DBConn.Table(ArchivedTable(table))
}

// ArchivedTable is the table, or on an archive node the view, that the queries of the API and of
// the synchronization of other nodes read for the rows of the table, wherever they are: a
// partitioned table or spent_info
func ArchivedTable(table string) string {
	if conf.Config.Storage.Archive {
		return fmt.Sprintf(`"%s"."%s_all"`, migration.ArchiveSchema, table)
	}
	return `"` + table + `"`
}
