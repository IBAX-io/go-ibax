/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"encoding/hex"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// copyInto copies the rows that rows gives to the columns cols of the table, on the connection of
// db, as the text of their values: COPY sends them as it reads them. It tells false and copies
// nothing when the connection is not one of the state, which the node opens all its connections as.
func copyInto(db *gorm.DB, table string, cols []string, rows func() []stateRow) (bool, error) {
	ran := false
	ctx := db.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	err := db.WithContext(withState(ctx, blockQueryer{func(ctx context.Context, c *stateConn) error {
		ran = true
		if c.inTx {
			if c.dirty == nil {
				c.dirty = make(map[string]bool)
			}
			c.dirty[table] = true
		}
		return copyTable(ctx, c, table, cols, rows())
	}})).Exec("SELECT 'copy into " + table + "'").Error
	return ran, err
}

// copyTable copies the rows to the columns cols of the table on the connection
func copyTable(ctx context.Context, c *stateConn, table string, cols []string, rows []stateRow) error {
	var b strings.Builder
	b.WriteString("COPY ")
	b.WriteString(quoteIdent(table))
	b.WriteString(" (")
	idx := make([]int, len(cols))
	for k, col := range cols {
		if k > 0 {
			b.WriteString(", ")
		}
		b.WriteString(quoteIdent(col))
		idx[k] = k
	}
	b.WriteString(") FROM STDIN")
	_, err := c.pg.Conn().PgConn().CopyFrom(ctx, &copyRows{cols: idx, rows: rows}, b.String())
	return err
}

// textBytea, textInt and textString are values in the text of their types
func textBytea(v []byte) *string {
	s := `\x` + hex.EncodeToString(v)
	return &s
}

func textInt(v int64) *string {
	s := strconv.FormatInt(v, 10)
	return &s
}

func textString(v string) *string { return &v }
