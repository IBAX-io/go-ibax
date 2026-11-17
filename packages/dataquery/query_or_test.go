/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package dataquery

import (
	"fmt"
	"slices"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/stretchr/testify/require"
)

// An $or of equalities of columns indexes start with reads each apart, and the pages are those of
// the plain filter: each row once, a row of both sides too, in the order, at every offset
func TestQueryReadsAnIndexedOrApart(t *testing.T) {
	countDB(t, 1)
	require.NoError(t, sqldb.DBConn.Exec(`CREATE TABLE "1_table1" (id bigint PRIMARY KEY, a bigint NOT NULL,
		b bigint NOT NULL, c bigint NOT NULL)`).Error)
	require.NoError(t, sqldb.DBConn.Exec(`CREATE INDEX ON "1_table1" (a, id)`).Error)
	require.NoError(t, sqldb.DBConn.Exec(`CREATE INDEX ON "1_table1" (b, id)`).Error)
	require.NoError(t, sqldb.DBConn.Exec(`INSERT INTO "1_table1" SELECT g, g % 3, g % 5, g % 2
		FROM generate_series(1, 200) g`).Error)
	table, err := Open(Reader{KeyID: 42, Ecosystem: 1}, "table1", 1)
	require.NoError(t, err)

	filter := func(text string) Where {
		var w Where
		require.NoError(t, DecodeJSON([]byte(text), &w))
		return w
	}
	split := func(w Where) bool {
		_, alternatives, err := table.where(&statement{schema: table}, w)
		require.NoError(t, err)
		return alternatives != nil
	}
	require.True(t, split(filter(`{"$or": [{"a": 1}, {"b": 1}], "c": 0}`)))
	require.True(t, split(filter(`{"$or": [{"a": 1}, {"b": 1}, {"id": 7}]}`)))
	// a column no index starts with, an operator, a null or a single filter: the plain filter
	require.False(t, split(filter(`{"$or": [{"a": 1}, {"c": 1}]}`)))
	require.False(t, split(filter(`{"$or": [{"a": 1}, {"b": {"$gt": 1}}]}`)))
	require.False(t, split(filter(`{"$or": [{"a": 1}, {"b": null}]}`)))
	require.False(t, split(filter(`{"$or": [{"a": 1, "c": 0}, {"b": 1}]}`)))
	require.False(t, split(filter(`{"$or": [{"a": 1}]}`)))

	// the ids of the rows of the plain filter, in the order
	want := func(match func(id int) bool, desc bool) []string {
		var ids []string
		for id := 1; id <= 200; id++ {
			if match(id) {
				ids = append(ids, fmt.Sprint(id))
			}
		}
		if desc {
			slices.Reverse(ids)
		}
		return ids
	}
	no := false
	for _, c := range []struct {
		where string
		order string
		match func(id int) bool
	}{
		{`{"$or": [{"a": 1}, {"b": 1}]}`, "desc", func(id int) bool { return id%3 == 1 || id%5 == 1 }},
		{`{"$or": [{"a": 1}, {"b": 1}], "c": 0}`, "asc", func(id int) bool { return (id%3 == 1 || id%5 == 1) && id%2 == 0 }},
		{`{"$or": [{"a": 2}, {"b": 2}, {"id": 30}], "id": {"$lt": 150}}`, "desc",
			func(id int) bool { return (id%3 == 2 || id%5 == 2 || id == 30) && id < 150 }},
	} {
		all := want(c.match, c.order == "desc")
		for _, limit := range []int{1, 7, 1000} {
			for offset := 0; offset <= len(all)+1; offset += max(1, limit/2) {
				result, err := table.Query(Query{Columns: []string{"id"}, Where: filter(c.where),
					Order: Order{{"id": c.order}}, Limit: &limit, Offset: offset, Count: &no})
				require.NoError(t, err)
				ids := []string{}
				for _, row := range result.List {
					ids = append(ids, fmt.Sprint(row["id"]))
				}
				page := all[min(offset, len(all)):min(offset+limit, len(all))]
				require.Equal(t, page, ids, "%s limit %d offset %d", c.where, limit, offset)
				require.Equal(t, offset+limit < len(all), result.More, "%s limit %d offset %d", c.where, limit, offset)
			}
		}
		count, err := table.Query(Query{Where: filter(c.where), Limit: new(int(1))})
		require.NoError(t, err)
		require.Equal(t, int64(len(all)), *count.Count, c.where)
	}
}
