/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// TestPermColumnOwnEcosystem adds column permissions on a running chain to a table with a row in
// 1_tables for each of three ecosystems, as keys has one for each ecosystem. The row of the caller's
// ecosystem is not the first of the table, as on a chain whose rows an update has moved: the row
// found by the name alone was another ecosystem's, whose columns were written with the one column
// over the caller's row, so that a second permission was lost. Each add must leave the rows of the
// other ecosystems byte-identical.
func TestPermColumnOwnEcosystem(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	pc, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	saved := sqldb.DBConn
	require.NoError(t, sqldb.GormInit(conf.DBConfig{Host: pc.Host, Port: int(pc.Port), User: pc.User, Name: pc.Database,
		Password: pc.Password, LockTimeout: 120000, IdleInTxTimeout: 150000, MaxIdleConns: 4, MaxOpenConns: 8}))
	t.Cleanup(func() { sqldb.GormClose(); sqldb.DBConn = saved })

	const name = "permcol_probe"
	exec := func(q string, args ...any) {
		require.NoError(t, sqldb.DBConn.Exec(q, args...).Error, q)
	}
	exec(`DELETE FROM "1_tables" WHERE name = ?`, name)
	t.Cleanup(func() { sqldb.DBConn.Exec(`DELETE FROM "1_tables" WHERE name = ?`, name) })
	// ecosystem 2 first, so that the first row of the name is not the caller's; then the caller's row
	// is updated, which puts its new version after the others
	for _, r := range []struct {
		eco     int64
		columns string
	}{{2, `{"x": "false", "y": "true"}`}, {1, `{"a": "true"}`}, {3, `{"z": "ContractAccess(\"@1Nobody\")"}`}} {
		exec(`INSERT INTO "1_tables" (id, name, permissions, columns, conditions, app_id, ecosystem)
			VALUES (next_id('1_tables'), ?, '{"read": "true"}', ?, 'true', 1, ?)`, name, r.columns, r.eco)
	}
	exec(`UPDATE "1_tables" SET conditions = 'true' WHERE name = ? AND ecosystem = 1`, name)

	tr, err := sqldb.StartTransaction()
	require.NoError(t, err)
	defer tr.Rollback()
	require.NoError(t, syspar.SysTableColType(tr))
	row := func(eco int64) string {
		var s string
		require.NoError(t, sqldb.GetDB(tr).Raw(`SELECT to_jsonb(t)::text FROM "1_tables" t WHERE name = ? AND ecosystem = ?`,
			name, eco).Row().Scan(&s))
		return s
	}
	columns := func(eco int64) string {
		var s string
		require.NoError(t, sqldb.GetDB(tr).Raw(`SELECT columns::text FROM "1_tables" WHERE name = ? AND ecosystem = ?`,
			name, eco).Row().Scan(&s))
		return s
	}
	sc := func(eco int64) *SmartContract {
		return &SmartContract{
			DbTransaction: tr,
			VM:            script.GetVM(),
			Key:           &sqldb.Key{},
			BlockHeader:   &types.BlockHeader{BlockId: 1},
			TxSmart:       &types.SmartTransaction{Header: &types.Header{EcosystemID: eco}},
		}
	}
	add := func(eco int64, column, perm string) {
		others := map[int64]string{}
		for _, e := range []int64{1, 2, 3} {
			if e != eco {
				others[e] = row(e)
			}
		}
		require.NoError(t, setColumnPermission(sc(eco), name, column, perm))
		for e, before := range others {
			require.Equal(t, before, row(e), fmt.Sprintf("the row of ecosystem %d after an add in ecosystem %d", e, eco))
		}
	}

	add(1, "b", "true")
	add(1, "c", "false")
	require.JSONEq(t, `{"a": "true", "b": "true", "c": "false"}`, columns(1), "two adds in ecosystem 1 both kept")
	add(2, "w", "true")
	require.JSONEq(t, `{"x": "false", "y": "true", "w": "true"}`, columns(2), "an add in ecosystem 2")
	require.JSONEq(t, `{"a": "true", "b": "true", "c": "false"}`, columns(1), "ecosystem 1 after the add in ecosystem 2")
	require.JSONEq(t, `{"z": "ContractAccess(\"@1Nobody\")"}`, columns(3), "ecosystem 3 untouched")
}
