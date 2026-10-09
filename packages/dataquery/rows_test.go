/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package dataquery

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A table of ecosystem 2 with a column no one may read, as a reader of ecosystem 3 opens it
func scopedTable(shared bool) *Table {
	return &Table{
		reader:    Reader{KeyID: 42, AccountID: "0000-0000-0000-0000-0042", Ecosystem: 3},
		Name:      "notes",
		Ecosystem: 2,
		shared:    shared,
		columns: map[string]tableColumn{
			"id": {t: TypeNumber}, "owner": {t: TypeText}, "key_id": {t: TypeNumber}, "meta": {t: TypeJSON},
		},
		// The owner's read condition, as evaluated: it does not hold
		checked: map[string]error{"owner": ErrAccessDenied("notes.owner")},
	}
}

func TestBindReaderPutsTheReadersValues(t *testing.T) {
	table := scopedTable(false)
	bound, err := table.bindReader(filter(t,
		`{"$or": [{"owner": "$account"}, {"key_id": {"$in": ["$key_id", 7]}}], "meta->eco": "$ecosystem_id", "name": "x$account"}`))
	require.NoError(t, err)
	assert.Equal(t, Where{
		"$or": []any{
			map[string]any{"owner": "0000-0000-0000-0000-0042"},
			map[string]any{"key_id": map[string]any{"$in": []any{json.Number("42"), json.Number("7")}}},
		},
		"meta->eco": json.Number("3"),
		"name":      "x$account",
	}, bound)

	_, err = table.bindReader(filter(t, `{"owner": "$owner"}`))
	assert.Equal(t, "E_VALUE", code(err))
}

func TestVisibleScopesEveryRead(t *testing.T) {
	table := scopedTable(true)
	s := &statement{schema: table}
	sql, err := table.visible(s)
	require.NoError(t, err)
	assert.Equal(t, `"ecosystem" = 2`, sql)
	assert.Empty(t, s.args)

	// The scope names a column the reader may not read, which the reader's own filter may not
	table.rows = Where{"owner": "0000-0000-0000-0000-0042"}
	sql, err = table.visible(s)
	require.NoError(t, err)
	assert.Equal(t, `"ecosystem" = 2 AND ("owner" = CAST($1 AS text))`, sql)
	assert.Equal(t, []any{"0000-0000-0000-0000-0042"}, s.args)
	_, err = s.where(Where{"owner": "x"}, 0)
	assert.Equal(t, "E_ACCESS_DENIED", code(err))

	// A scope naming what the table lacks reads no rows
	table.rows = Where{"missing": "x"}
	_, err = table.visible(&statement{schema: table})
	assert.Equal(t, "E_COLUMN", code(err))
}

func TestReadRowScopeRefusesABrokenScope(t *testing.T) {
	for _, text := range []string{`not json`, `{"owner": "$who"}`, `{"missing": 1}`, `{"owner": {"$near": 1}}`} {
		table := scopedTable(false)
		table.Record.Permissions.Rows = text
		assert.Equal(t, "E_ACCESS_DENIED", code(table.readRowScope()), text)
	}
	table := scopedTable(false)
	assert.NoError(t, table.readRowScope())
	assert.Nil(t, table.rows)
	table.Record.Permissions.Rows = `{"key_id": "$key_id"}`
	require.NoError(t, table.readRowScope())
	assert.Equal(t, Where{"key_id": json.Number("42")}, table.rows)
}
