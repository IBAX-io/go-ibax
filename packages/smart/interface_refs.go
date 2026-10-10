/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	qb "github.com/IBAX-io/go-ibax/packages/storage/sqldb/queryBuilder"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/wtl"
)

// interfaceTables are the tables of the elements of the interface, whose refs column holds the
// elements their value refers to (see package wtl)
var interfaceTables = map[string]bool{"pages": true, "menu": true, "snippets": true}

// refsKinds are the kinds of the elements a value refers to, and the tables that keep them
var refsKinds = map[string]string{"blocks": "snippets", "pages": "pages"}

// setInterfaceRefs keeps the refs of a row of an interface table the function of its value:
// whenever a value is written, its refs are written with it. The refs are never written
// otherwise.
func setInterfaceRefs(b *qb.SQLQueryBuilder) error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if !interfaceTables[b.KeyTable()] {
		return nil
	}
	if _, ok := b.FieldValue("refs"); ok {
		return errRefsWrite
	}
	value, ok := b.FieldValue("value")
	if !ok {
		return nil
	}
	b.AddField("refs", wtl.Read(value, converter.StrToInt64(b.GetEcosystem())).JSON())
	return nil
}

// InterfaceReferrers returns the pages, menus and blocks that refer to an element of the
// ecosystem: kind is "blocks" for a block and "pages" for a page. Each item is a map of the
// type (the table: pages, menu or snippets), the ecosystem and the name of an element, in that
// order; the element itself is not one. The elements of other ecosystems refer to it by its
// name with the prefix of its ecosystem.
func InterfaceReferrers(sc *SmartContract, kind, name string) ([]any, error) {
	own, ok := refsKinds[kind]
	if !ok {
		return nil, fmt.Errorf(`unknown kind of element %s`, kind)
	}
	if len(name) == 0 {
		return nil, fmt.Errorf(`the name of the element is empty`)
	}
	ecosystem := sc.TxSmart.EcosystemID
	contains := func(ref string) (string, error) {
		out, err := json.Marshal(map[string][]string{kind: {ref}})
		return string(out), err
	}
	inOwn, err := contains(name)
	if err != nil {
		return nil, err
	}
	inOther, err := contains(fmt.Sprintf(`@%d%s`, ecosystem, name))
	if err != nil {
		return nil, err
	}
	var (
		query string
		args  []any
	)
	for _, table := range []string{"menu", "pages", "snippets"} {
		if len(query) > 0 {
			query += ` UNION ALL `
		}
		query += fmt.Sprintf(`SELECT '%[1]s' AS type, ecosystem, name FROM "1_%[1]s"
			WHERE ((ecosystem = ? AND refs @> ?::jsonb) OR (ecosystem <> ? AND refs @> ?::jsonb))`, table)
		args = append(args, ecosystem, inOwn, ecosystem, inOther)
		if table == own {
			query += ` AND NOT (ecosystem = ? AND name = ?)`
			args = append(args, ecosystem, name)
		}
	}
	query += fmt.Sprintf(` ORDER BY type, ecosystem, name LIMIT %d`, consts.DBFindLimit)
	rows, err := sc.DbTransaction.GetAllTransaction(query, -1, args...)
	if err != nil {
		return nil, logErrorDB(err, "selecting the referrers of an element")
	}
	result := make([]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, types.LoadMap(map[string]any{
			"type":      row["type"],
			"ecosystem": converter.StrToInt64(row["ecosystem"]),
			"name":      row["name"],
		}))
	}
	return result, nil
}

// DBDelete deletes the row with the id from the table. A table is deleted from only when its
// permissions say who may: the delete permission is evaluated as the others are, and a table
// without one refuses deleting. The row is kept in the rollback of the transaction.
func DBDelete(sc *SmartContract, tblname string, id int64) (qcost int64, err error) {
	if tblname == "platform_parameters" {
		return 0, fmt.Errorf("platform parameters access denied")
	}
	tblname = qb.GetTableName(sc.TxSmart.EcosystemID, tblname)
	perm, err := sc.AccessTablePerm(tblname, "delete")
	if err != nil {
		return 0, err
	}
	if len(perm["delete"]) == 0 {
		return 0, fmt.Errorf("table %s: %w", tblname, errDeleteUndeclared)
	}
	return sc.deleteRow(tblname, id, !sc.ChildChain && sc.Rollback)
}

// SysRollbackDeleteRow inserts again the row a transaction deleted
func SysRollbackDeleteRow(dbTx *sqldb.DbTransaction, sysData SysRollData) error {
	table := `"` + sysData.TableName + `"`
	err := dbTx.ExecSql(`INSERT INTO ` + table + ` SELECT * FROM jsonb_populate_record(NULL::` + table +
		`, '` + strings.ReplaceAll(sysData.Data, `'`, `''`) + `'::jsonb)`)
	if err != nil {
		return logErrorDB(err, "inserting the deleted row")
	}
	return nil
}
