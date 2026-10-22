/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package dataquery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The table definitions have no row of their own in 1_tables, yet every reader reads them, each
// within the ecosystem it names, and no row write may change them
func TestTheTableDefinitionsAreReadByEveryone(t *testing.T) {
	table := &Table{reader: Reader{KeyID: 42, Ecosystem: 3}, Name: "tables", Ecosystem: 2, shared: true}
	require.NoError(t, table.ownRecord())

	perm := table.Record.Permissions
	assert.Equal(t, "true", perm.Read)
	assert.Empty(t, perm.Rows)
	assert.Empty(t, table.Record.Columns)
	assert.Equal(t, int64(2), table.Record.Ecosystem)
	for action, cond := range map[string]string{
		"insert": perm.Insert, "update": perm.Update, "new_column": perm.NewColumn, "conditions": table.Record.Conditions,
	} {
		assert.Equal(t, "false", cond, action)
	}

	require.NoError(t, table.readConditions())
	assert.Empty(t, table.reads)
	require.NoError(t, table.readRowScope())
	sql, err := table.visible(&statement{schema: table})
	require.NoError(t, err)
	assert.Equal(t, `"ecosystem" = 2`, sql)
}

// Any other table without a row in 1_tables belongs to the node itself
func TestTheNodesOwnTablesAreRefused(t *testing.T) {
	for _, name := range []string{"info_block", "transactions_status", "confirmations"} {
		table := &Table{Name: name, Ecosystem: 1}
		assert.Equal(t, "E_ACCESS_DENIED", code(table.ownRecord()), name)
	}
}
