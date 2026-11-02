/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package types

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// marshalConcat is the text MarshalJSON made of a map when it concatenated it
func marshalConcat(m *Map) ([]byte, error) {
	s := "{"
	for current := m.head; current != nil; current = current.next {
		s = s + `"` + strings.Replace(current.key, `"`, `\"`, -1) + `":`
		vBytes, err := json.Marshal(current.value)
		if err != nil {
			return []byte{}, err
		}
		s = s + string(vBytes) + ","
	}
	if len(s) > 1 {
		s = s[0 : len(s)-1]
	}
	return []byte(s + "}"), nil
}

// TestMapMarshalJSON: the text of a map is the text it was when MarshalJSON concatenated it
func TestMapMarshalJSON(t *testing.T) {
	nested := NewMap()
	nested.Set("value", decimal.RequireFromString("12.5"))
	nested.Set("flag", int64(2))
	maps := []*Map{
		NewMap(),
		LoadMap(map[string]any{"a": 1}),
		LoadMap(map[string]any{`q"uote`: "x", `back\slash`: `y\z`, "html": "<a&b>", "nil": nil, "list": []any{1, "two", nested}}),
		LoadMap(map[string]any{"nested": nested, "f": 1.5, "b": []byte("bytes"), "t": true}),
	}
	for _, m := range maps {
		want, err := marshalConcat(m)
		require.NoError(t, err)
		got, err := m.MarshalJSON()
		require.NoError(t, err)
		require.Equal(t, string(want), string(got))
		// as a value of other JSON too, which encoding/json compacts and escapes
		// (a key with a backslash is no JSON, either way)
		wantJSON, wantErr := json.Marshal(json.RawMessage(want))
		gotJSON, gotErr := json.Marshal(m)
		require.Equal(t, wantErr != nil, gotErr != nil, string(want))
		require.Equal(t, string(wantJSON), string(gotJSON))
	}
}
