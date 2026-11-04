/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestKnownLogTransactions checks the hashes it finds in parts are those log_transactions has
func TestKnownLogTransactions(t *testing.T) {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	conn, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	saved := DBConn
	DBConn = conn
	t.Cleanup(func() { DBConn = saved })

	var hashes, logged [][]byte
	for i := range 3*knownChunk + 7 {
		h := []byte(fmt.Sprintf("known-log-test-%d", i))
		hashes = append(hashes, h)
		if i%5 == 0 {
			logged = append(logged, h)
		}
	}
	arr := column(logged, func(h []byte) []byte { return h })
	del := func() {
		conn.Exec(`DELETE FROM "log_transactions" WHERE "block" = 1 AND "hash" = ANY(?::bytea[])`, arr)
	}
	del()
	t.Cleanup(del)
	require.NoError(t, conn.Exec(`INSERT INTO "log_transactions" ("hash", "block") SELECT unnest(?::bytea[]), 1`, arr).Error)

	known, err := KnownLogTransactions(nil, hashes)
	require.NoError(t, err)
	require.Len(t, known, len(logged))
	for _, h := range logged {
		require.True(t, known[string(h)])
	}
	empty, err := KnownLogTransactions(nil, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}
