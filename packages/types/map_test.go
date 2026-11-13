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

// FuzzMapSize checks a map of NewMapSize against one of NewMap under the same operations
func FuzzMapSize(f *testing.F) {
	f.Add(uint8(3), []byte{0, 1, 2, 0x41, 0x82, 3, 0xc0, 4})
	f.Add(uint8(0), []byte{1, 1, 0x81, 1})
	f.Fuzz(func(t *testing.T, size uint8, ops []byte) {
		a, b := NewMap(), NewMapSize(int(size%16))
		for i, op := range ops {
			key := string(rune('a' + op%8))
			switch op >> 6 {
			case 0, 1:
				a.Set(key, i)
				b.Set(key, i)
			case 2:
				a.Remove(key)
				b.Remove(key)
			case 3:
				if op%8 == 0 {
					a.Clear()
					b.Clear()
				}
			}
			require.Equal(t, a.Keys(), b.Keys())
			require.Equal(t, a.Values(), b.Values())
			require.Equal(t, a.String(), b.String())
			require.Equal(t, a.Size(), b.Size())
			ja, erra := a.MarshalJSON()
			jb, errb := b.MarshalJSON()
			require.Equal(t, string(ja), string(jb))
			require.Equal(t, erra, errb)
			for l, m := a.First(), b.First(); l != nil || m != nil; l, m = l.Next(), m.Next() {
				require.Equal(t, l.Key(), m.Key())
				require.Equal(t, l.Value(), m.Value())
			}
		}
	})
}
