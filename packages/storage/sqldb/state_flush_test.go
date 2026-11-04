/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

// TestStateFlushInsert: the rows the state copies to a table are, value by value, the texts
// PostgreSQL output for them, the characters COPY escapes and NULL included
func TestStateFlushInsert(t *testing.T) {
	conn := testConn(t)
	begin(t, conn)
	ctx := context.Background()
	meta, types := probeTable(t, conn)
	extra := map[string][]string{
		"text":    {`\N`, `a\b`, "tab\there", "line\nbreak", "cr\rret", `\\`, `\.`, "end\\"},
		"varchar": {`\N`, "x\ty", `back\slash`},
		"jsonb":   {`{"k":"a\\b\tc\nd"}`},
	}
	var cols []string
	values := make([][]*string, len(types))
	for i, typ := range types {
		col := "c" + strconv.Itoa(i)
		cols = append(cols, col)
		for _, in := range slices.Concat(typeInputs[typ], extra[typ]) {
			if text, _, ok := pgAssign(t, conn, col, in); ok {
				values[i] = append(values[i], &text)
			}
		}
		values[i] = append(values[i], nil)
	}
	n := 0
	for _, v := range values {
		n = max(n, len(v))
	}
	var rows []stateRow
	want := make(map[int][]*string)
	idCol := meta.colIndex("id")
	for r := range n {
		row := make(stateRow, len(meta.cols))
		id := strconv.Itoa(r + 2)
		row[idCol] = &id
		for i, col := range cols {
			v := values[i][r%len(values[i])]
			row[meta.colIndex(col)] = v
			want[r+2] = append(want[r+2], v)
		}
		rows = append(rows, row)
	}
	all := make([]int, len(meta.cols))
	for i := range all {
		all[i] = i
	}
	require.NoError(t, conn.Raw(func(dc any) error {
		return flushInsert(ctx, &stateConn{pg: dc.(*stdlib.Conn)}, meta, all, rows)
	}))
	for id, vals := range want {
		for i, col := range cols {
			got := pgText(t, conn, fmt.Sprintf(`SELECT "%s" FROM state_probe WHERE id = %d`, col, id))
			require.Equal(t, vals[i], got, "%s %s row %d", types[i], col, id)
		}
	}
}

// TestCopyRows: the rows read in pieces of any length are the text of COPY of each row
func TestCopyRows(t *testing.T) {
	s := func(v string) *string { return &v }
	rows := []stateRow{
		{s("1"), s("plain"), nil},
		{s("2"), s("a\\b\tc\nd\re"), s("")},
		{s("3"), s("\\"), s("end\\")},
	}
	want := "1\tplain\t\\N\n2\ta\\\\b\\tc\\nd\\re\t\n3\t\\\\\tend\\\\\n"
	for _, size := range []int{1, 2, 7, 64, 65536} {
		r := &copyRows{cols: []int{0, 1, 2}, rows: slices.Clone(rows)}
		var got []byte
		buf := make([]byte, size)
		for {
			n, err := r.Read(buf)
			got = append(got, buf[:n]...)
			if err != nil {
				require.Equal(t, io.EOF, err)
				break
			}
		}
		require.Equal(t, want, string(got), "pieces of %d", size)
	}
}
