/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"strings"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/migration"
	"github.com/stretchr/testify/require"
)

// TestClosedRanges: the ranges below the active one are closed, whatever their table; the active
// range, the next, the partitions that never leave and names of no range are not
func TestClosedRanges(t *testing.T) {
	names := []string{"rollback_tx_p0", "rollback_tx_p1", "rollback_tx_p2", "1_history_p1", "block_chain_genesis",
		"spent_info_unspent", "spent_info_p0", "log_transactions_p3", "log_transactions_px"}
	require.Equal(t, []string{"rollback_tx_p0", "rollback_tx_p1", "1_history_p1", "spent_info_p0"},
		closedRanges(names, 2))
	require.Empty(t, closedRanges(names, 0))
}

// TestPartitionOptions: a range partition does not vacuum on inserts, nor does its TOAST table
func TestPartitionOptions(t *testing.T) {
	db := testDB(t)
	require.NoError(t, db.Exec(migration.CreatePartitionSQL("rollback_tx", 7)).Error)
	var opts []string
	require.NoError(t, db.Raw(`SELECT array_to_string(c.reloptions, ',') || '|' || coalesce(array_to_string(x.reloptions, ','), '')
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace LEFT JOIN pg_class x ON x.oid = c.reltoastrelid
		WHERE n.nspname = ? AND c.relname = ?`, migration.PartsSchema, "rollback_tx_p7").Scan(&opts).Error)
	require.Len(t, opts, 1)
	main, toast, _ := strings.Cut(opts[0], "|")
	require.Equal(t, "autovacuum_vacuum_insert_threshold=-1", main)
	require.Equal(t, "autovacuum_vacuum_insert_threshold=-1", toast)
}
