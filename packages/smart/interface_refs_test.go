/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"errors"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	qb "github.com/IBAX-io/go-ibax/packages/storage/sqldb/queryBuilder"
	"github.com/IBAX-io/go-ibax/packages/types"
)

func TestSetInterfaceRefs(t *testing.T) {
	const page = "@wtl 1\n<Page><Include name=\"@5own\" /><Include name=\"@1shared\" /></Page>\n"
	for _, c := range []struct {
		name, table string
		ecosystem   int64
		fields      []string
		values      []any
		where       *types.Map
		refs        string
		err         error
	}{
		{"insert of a page", "5_pages", 5, []string{"name", "value"}, []any{"p", page}, nil,
			`{"blocks":["@1shared","own"]}`, nil},
		{"update of a block", "5_snippets", 5, []string{"value"}, []any{page}, types.LoadMap(map[string]any{"id": 3}),
			`{"blocks":["@1shared","own"]}`, nil},
		{"a legacy template refers to nothing", "1_menu", 1, []string{"value"}, []any{"MenuItem(Page: x)"}, nil, `{}`, nil},
		{"no value, no refs", "5_pages", 5, []string{"menu"}, []any{"m"}, types.LoadMap(map[string]any{"id": 3}), "", nil},
		{"another table", "5_languages", 5, []string{"value"}, []any{page}, nil, "", nil},
		{"refs written by the caller", "5_pages", 5, []string{"value", "refs"}, []any{page, `{}`}, nil, "", errRefsWrite},
	} {
		b := &qb.SQLQueryBuilder{Table: c.table, Fields: c.fields, FieldValues: c.values, Where: c.where,
			TxEcoID: c.ecosystem, KeyTableChkr: sqldb.KeyTableChecker{}}
		if err := setInterfaceRefs(b); !errors.Is(err, c.err) {
			t.Errorf("%s: error %v, want %v", c.name, err, c.err)
			continue
		}
		if c.err != nil {
			continue
		}
		refs, ok := b.FieldValue("refs")
		if c.refs == "" && ok || c.refs != "" && refs != c.refs {
			t.Errorf("%s: refs %q (%v), want %q", c.name, refs, ok, c.refs)
		}
	}
}
