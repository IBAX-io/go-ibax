/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetColumnsIndexes(t *testing.T) {
	_, _, indexes, err := getColumns(`[
		{"name":"ecosystem1","type":"number","conditions":"true","unique":"pair"},
		{"name":"ecosystem2","type":"number","conditions":"true","unique":"pair"},
		{"name":"owner","type":"number","conditions":"true","index":"owner"},
		{"name":"pair_id","type":"number","conditions":"true","index":"owner"},
		{"name":"topic","type":"varchar","conditions":"true"}
	]`)
	require.NoError(t, err)
	require.Len(t, indexes, 2)
	require.Equal(t, tableIndex{name: "pair", unique: true, cols: []string{"ecosystem1", "ecosystem2"}}, *indexes[0])
	require.Equal(t, tableIndex{name: "owner", unique: false, cols: []string{"owner", "pair_id"}}, *indexes[1])

	for _, columns := range []string{
		// an index of a column of a type an index does not cover
		`[{"name":"topic","type":"varchar","conditions":"true","unique":"topic"}]`,
		// a name that is not valid
		`[{"name":"a","type":"number","conditions":"true","unique":"Pair"}]`,
		`[{"name":"a","type":"number","conditions":"true","unique":1}]`,
		// one name for a unique index and an index that is not unique
		`[{"name":"a","type":"number","conditions":"true","unique":"x"},{"name":"b","type":"number","conditions":"true","index":"x"}]`,
	} {
		_, _, _, err = getColumns(columns)
		require.Error(t, err, columns)
	}
}
