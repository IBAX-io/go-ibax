/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMaxIDStmt checks the select of the highest id that MaxID sends plans as the read of the
// highest id, of a table whose ids grow with its key as of any other: planned as a select, it
// sorts every row of the table in memory at each insert
func TestMaxIDStmt(t *testing.T) {
	cat := &stateCatalog{rels: map[string]bool{"1_history": true, "1_keys": true, "rollback_tx": true}}
	for _, c := range []struct {
		query string
		max   bool
	}{
		{`select id from "1_keys" order by id desc limit 1`, true},
		{`select id from "1_history" order by "block_id" desc, id desc limit 1`, true},
		{`select id from "rollback_tx" order by "block_id" desc, id desc limit 1`, true},
		// the order of another key, or ascending, finds another row first
		{`select id from "1_keys" order by "ecosystem" desc, id desc limit 1`, false},
		{`select id from "1_history" order by "block_id" asc, id desc limit 1`, false},
		{`select id from "1_history" order by "block_id" desc, id asc limit 1`, false},
		{`select id from "1_history" order by "block_id" desc, id desc limit 2`, false},
	} {
		st := parseStmt(c.query, cat)
		require.Equal(t, c.max, st.kind == stMaxID, c.query)
	}
}
