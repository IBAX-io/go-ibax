/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"os"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestFeeKeyIgnoresTablePermissions: the fee payment adds the account of the block producer in a
// transaction whose contract the keys table does not let insert, as @1ImportUpload
func TestFeeKeyIgnoresTablePermissions(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	tx := conn.Begin()
	require.NoError(t, tx.Error)
	saved := sqldb.DBConn
	sqldb.DBConn = tx
	t.Cleanup(func() {
		sqldb.DBConn = saved
		tx.Rollback()
	})

	InitVM()
	const id = int64(-2338305592748021477)
	sc := &SmartContract{
		DbTransaction: sqldb.NewDbTransaction(tx),
		Key:           &sqldb.Key{},
		VM:            script.GetVM(),
		TxContract:    &Contract{Name: "@1ImportUpload", StackCont: []any{"@1ImportUpload"}},
		TxSmart:       &types.SmartTransaction{Header: &types.Header{EcosystemID: 1}},
	}
	require.ErrorIs(t, sc.AccessTable("1_keys", "insert"), errAccessDenied,
		"the contract may not insert into the keys table")

	require.NoError(t, sc.hasExistKeyID(1, id))
	require.False(t, sc.FullAccess)
	found, err := (&sqldb.Key{}).SetTablePrefix(1).Get(sc.DbTransaction, id)
	require.NoError(t, err)
	require.True(t, found)
}
