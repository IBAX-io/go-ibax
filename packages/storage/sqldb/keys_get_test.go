/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestFirstKey: the key firstKey reads is the key gorm's First reads into the same struct, found or
// not, with null columns, with an id set already, and the statement is the same
func TestFirstKey(t *testing.T) {
	tx := testDB(t)
	require.NoError(t, tx.Exec(`CREATE TEMP TABLE "1_keys" (id bigint, pub bytea, multi bigint, amount numeric(30),
		maxpay numeric(30), deleted bigint, blocked bigint, ecosystem bigint, account char(24), conditions text) ON COMMIT DROP`).Error)
	require.NoError(t, tx.Exec(`INSERT INTO "1_keys" VALUES
		(5, '\x0102', 0, 120, 7, 0, 1, 1, '0000-0000-0000-0000-0005', 'C'),
		(6, NULL, NULL, NULL, NULL, NULL, NULL, 1, NULL, NULL),
		(5, '\x03', 0, 9, 0, 1, 0, 2, '0000-0000-0000-0000-0005', '')`).Error)
	dbTx := NewDbTransaction(tx)
	prefilled := func(eco, id int64) *Key {
		return &Key{ecosystem: eco, ID: id, AccountID: "a", PublicKey: []byte{9}, Amount: "1", Maxpay: "2",
			Deleted: 3, Blocked: 4, Conditions: "x"}
	}
	for _, c := range []struct{ eco, id, wallet int64 }{
		{1, 0, 5}, {1, 0, 6}, {2, 0, 5}, {1, 0, 7}, {3, 0, 5}, {1, 5, 5}, {1, 6, 5}, {1, 6, 6},
	} {
		want := prefilled(c.eco, c.id)
		wantFound, wantErr := isFound(tx.Where("id = ? and ecosystem = ?", c.wallet, want.ecosystem).First(want))
		got := prefilled(c.eco, c.id)
		gotFound, gotErr := got.Get(dbTx, c.wallet)
		require.Equal(t, wantErr, gotErr, "%+v", c)
		require.Equal(t, wantFound, gotFound, "%+v", c)
		require.Equal(t, want, got, "%+v", c)
	}
	dry := tx.Session(&gorm.Session{DryRun: true})
	require.Equal(t, firstKeySQL, dry.Where("id = ? and ecosystem = ?", 5, 1).First(&Key{}).Statement.SQL.String())
	require.Equal(t, firstKeyByIDSQL, dry.Where("id = ? and ecosystem = ?", 5, 1).First(&Key{ID: 5}).Statement.SQL.String())
}
