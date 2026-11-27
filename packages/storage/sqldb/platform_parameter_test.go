/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPlatformParameterHistory: the history of a parameter is the old values of the rollback
// records of its row from the block on, the latest first; the records of other rows, of other
// tables, of earlier blocks and without a value are not of it
func TestPlatformParameterHistory(t *testing.T) {
	db := retentionDB(t)
	tx := GetDB(db)
	var id, other int64
	require.NoError(t, tx.Raw(`SELECT "id" FROM "1_platform_parameters" WHERE "name" = 'honor_nodes'`).Scan(&id).Error)
	require.NoError(t, tx.Raw(`SELECT "id" FROM "1_platform_parameters" WHERE "name" <> 'honor_nodes' ORDER BY "id" LIMIT 1`).Scan(&other).Error)
	row := func(block int64, table string, tableID int64, data string) *RollbackTx {
		return &RollbackTx{BlockID: block, NameTable: table, TableID: strconv.FormatInt(tableID, 10), Data: data}
	}
	require.NoError(t, CreateBatchesRollbackTx(tx, []*RollbackTx{
		row(3, "1_platform_parameters", id, `{"value":"v3"}`),
		row(5, "1_platform_parameters", id, `{"value":"v5"}`),
		row(5, "1_platform_parameters", other, `{"value":"other"}`),
		row(6, "1_keys", id, `{"value":"keys"}`),
		row(7, "1_platform_parameters", id, `{"conditions":"c"}`),
		row(8, "1_platform_parameters", id, `{"value":"v8a"}`),
		row(8, "1_platform_parameters", id, `{"value":"v8b"}`),
	}, NewTableRefs()))

	values, err := PlatformParameterHistory(db, "honor_nodes", 5)
	require.NoError(t, err)
	require.Equal(t, []string{"v8b", "v8a", "v5"}, values)
	values, err = PlatformParameterHistory(db, "honor_nodes", 9)
	require.NoError(t, err)
	require.Empty(t, values)
}
