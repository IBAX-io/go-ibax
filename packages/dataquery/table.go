/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package dataquery is the one implementation of the node's data reads, for the REST API and
// JSON-RPC alike (docs/api/data-query.md): a table is read only as its read conditions allow the
// reader, every value of a query is bound, every name it holds is one of the table's columns, and
// every cell comes typed.
package dataquery

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"
)

// Reader is who reads: the key and the ecosystem of the session
type Reader struct {
	KeyID     int64
	AccountID string
	Ecosystem int64
}

type tableColumn struct {
	t        Type
	withZone bool
}

// Table is a table of an ecosystem as one reader may read it
type Table struct {
	reader Reader
	// Name is the table's name in its ecosystem, Ecosystem that ecosystem
	Name      string
	Ecosystem int64
	// Record is the table's row of 1_tables: its permissions and conditions
	Record   sqldb.Table
	physical string
	// A table of ecosystem 1 that holds the rows of every ecosystem, in an ecosystem column
	shared  bool
	columns map[string]tableColumn
	ordered []string
	reads   map[string]string
	checked map[string]error
}

var prefixedName = regexp.MustCompile(`^@([1-9][0-9]*)([a-z_][a-z0-9_]*)$`)

// Open finds a table for a reader, and checks that the reader may read it
func Open(reader Reader, name string, ecosystem int64) (*Table, error) {
	t, err := Describe(reader, name, ecosystem)
	if err != nil {
		return nil, err
	}
	if cond := t.Record.Permissions.Read; cond != "" && !t.holds(cond) {
		return nil, ErrAccessDenied(t.Name)
	}
	return t, nil
}

// Describe finds a table for a reader: its definition, which is public. The name may be prefixed
// with its ecosystem (@2orders); else the table is of the ecosystem given, else of the reader's.
func Describe(reader Reader, name string, ecosystem int64) (*Table, error) {
	name = strings.ToLower(name)
	if m := prefixedName.FindStringSubmatch(name); m != nil {
		ecosystem, _ = strconv.ParseInt(m[1], 10, 64)
		name = m[2]
	}
	if ecosystem <= 0 {
		ecosystem = reader.Ecosystem
	}
	if !identifier.MatchString(name) {
		return nil, ErrTableNotFound(name)
	}
	t := &Table{reader: reader, Name: name, Ecosystem: ecosystem, checked: map[string]error{}}
	t.shared = converter.FirstEcosystemTables[name]
	if t.shared {
		t.physical = "1_" + name
	} else {
		t.physical = fmt.Sprintf("%d_%s", ecosystem, name)
	}
	t.Record.SetTablePrefix(strconv.FormatInt(ecosystem, 10))
	found, err := t.Record.Get(nil, name)
	if err != nil {
		return nil, queryFailed(err, t.physical)
	}
	if err := t.readColumns(); err != nil {
		return nil, err
	}
	if !found {
		// A table of the node's own, not of an ecosystem: never read through the data API
		if len(t.ordered) > 0 {
			return nil, ErrAccessDenied(name)
		}
		return nil, ErrTableNotFound(name)
	}
	if len(t.ordered) == 0 {
		return nil, ErrTableNotFound(name)
	}
	if err := t.readConditions(); err != nil {
		return nil, err
	}
	return t, nil
}

func queryFailed(err error, table string) *Error {
	log.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).Error("reading data")
	return ErrQuery
}

func (t *Table) readColumns() error {
	rows, err := query(`SELECT column_name, data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1 ORDER BY ordinal_position`, t.physical)
	if err != nil {
		return queryFailed(err, t.physical)
	}
	defer rows.Close()
	t.columns = map[string]tableColumn{}
	for rows.Next() {
		var name, dataType string
		if err := rows.Scan(&name, &dataType); err != nil {
			return queryFailed(err, t.physical)
		}
		t.columns[name] = tableColumn{t: TypeOf(dataType), withZone: dataType == "timestamp with time zone"}
		t.ordered = append(t.ordered, name)
	}
	if err := rows.Err(); err != nil {
		return queryFailed(err, t.physical)
	}
	return nil
}

// The read condition of each column: 1_tables.columns holds per column either its update condition
// alone, or {"update": ..., "read": ...}, as an object or as its JSON text
func (t *Table) readConditions() error {
	t.reads = map[string]string{}
	if t.Record.Columns == "" {
		return nil
	}
	var columns map[string]json.RawMessage
	if err := json.Unmarshal([]byte(t.Record.Columns), &columns); err != nil {
		return queryFailed(err, t.physical)
	}
	for name, raw := range columns {
		var perm struct {
			Read string `json:"read"`
		}
		var text string
		if json.Unmarshal(raw, &text) == nil {
			if !strings.HasPrefix(strings.TrimSpace(text), "{") {
				continue
			}
			raw = json.RawMessage(text)
		}
		if err := json.Unmarshal(raw, &perm); err != nil {
			return queryFailed(err, t.physical)
		}
		if perm.Read != "" {
			t.reads[name] = perm.Read
		}
	}
	return nil
}

// holds evaluates a condition of the table for the reader, in the table's ecosystem; a condition
// that fails to evaluate does not hold
func (t *Table) holds(cond string) bool {
	sc := &smart.SmartContract{
		ChildChain: conf.Config.IsSupportingChildChain(),
		VM:         script.GetVM(),
		TxSmart: &types.SmartTransaction{
			Header: &types.Header{
				EcosystemID: t.Ecosystem,
				KeyID:       t.reader.KeyID,
				NetworkID:   conf.Config.LocalConf.NetworkID,
			},
		},
		Key: &sqldb.Key{ID: t.reader.KeyID, AccountID: t.reader.AccountID},
	}
	ok, err := sc.EvalIf(cond)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.EvalError, "error": err, "table": t.physical, "condition": cond}).
			Warning("evaluating a read condition")
		return false
	}
	return ok
}

func (t *Table) column(name string) (Type, bool, bool) {
	c, found := t.columns[name]
	return c.t, c.withZone, found
}

func (t *Table) readable(name string) error {
	if err, done := t.checked[name]; done {
		return err
	}
	var err error
	if cond, has := t.reads[name]; has && !t.holds(cond) {
		err = ErrAccessDenied(t.Name + "." + name)
	}
	t.checked[name] = err
	return err
}

// Columns are all the columns of the table, in their order, with their types
func (t *Table) Columns() []Column {
	columns := make([]Column, 0, len(t.ordered))
	for _, name := range t.ordered {
		columns = append(columns, Column{Name: name, Type: t.columns[name].t})
	}
	return columns
}

// Readable tells whether the reader may read a column of the table
func (t *Table) Readable(name string) bool {
	_, found := t.columns[name]
	return found && t.readable(name) == nil
}

// The rows of the table the reader's queries see: in a shared table, those of its ecosystem
func (t *Table) scope() string {
	if t.shared {
		return `"ecosystem" = ` + strconv.FormatInt(t.Ecosystem, 10)
	}
	return "TRUE"
}

// Physical is the name of the table in the database
func (t *Table) Physical() string {
	return t.physical
}

func (t *Table) from() string {
	return `"` + t.physical + `"`
}

// query runs a read with every value bound apart from the SQL text
func query(text string, args ...any) (*sql.Rows, error) {
	db, err := sqldb.DBConn.DB()
	if err != nil {
		return nil, err
	}
	return db.QueryContext(context.Background(), text, append([]any{pgx.QueryExecModeExec}, args...)...)
}
