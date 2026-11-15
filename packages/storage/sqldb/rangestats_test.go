/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"crypto/sha256"
	"encoding/binary"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/migration"
	"github.com/stretchr/testify/require"
)

// TestAnalyzeRange: the lookup of the hashes of a block expects most rows of a partition without
// statistics, and the hashes it looks up of an analyzed one; the node analyzes a partition again
// once it has twice the pages, and leaves it from rangeSettledPages; the empty ones wait
func TestAnalyzeRange(t *testing.T) {
	db := testDB(t)
	const k = 900
	for _, table := range migration.PartitionedTables {
		require.NoError(t, db.Exec(migration.CreatePartitionSQL(table.Name, k)).Error)
	}
	insert := func(from, to int) {
		require.NoError(t, db.Exec(`INSERT INTO "log_transactions" ("hash", "block")
			SELECT sha256(int8send(i)), ? + i % ? FROM generate_series(?::bigint, ?::bigint) i`,
			k*migration.PartitionBlocks, migration.PartitionBlocks, from, to).Error)
	}
	insert(1, 12000)
	hashes := make([][]byte, 512)
	for i := range hashes {
		h := sha256.Sum256(binary.BigEndian.AppendUint64(nil, uint64(i*11+1)))
		hashes[i] = h[:]
	}
	name := migration.PartitionName("log_transactions", k)
	rowsOf := regexp.MustCompile(` rows=(\d+) `)
	expected := func() int {
		var plan []string
		require.NoError(t, db.Raw(`EXPLAIN SELECT "hash" FROM "log_transactions" WHERE "hash" = ANY(?::bytea[])`,
			column(hashes, func(h []byte) []byte { return h })).Scan(&plan).Error)
		for _, line := range plan {
			if strings.Contains(line, " on "+name+" ") {
				m := rowsOf.FindStringSubmatch(line)
				require.NotNil(t, m, line)
				n, err := strconv.Atoi(m[1])
				require.NoError(t, err)
				return n
			}
		}
		t.Fatalf("no scan of %s in %q", name, plan)
		return 0
	}
	pages := func() (analyzed, now int64) {
		require.NoError(t, db.Raw(`SELECT c.relpages, pg_relation_size(c.oid) / current_setting('block_size')::bigint
			FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = ? AND c.relname = ?`,
			migration.PartsSchema, name).Row().Scan(&analyzed, &now))
		return
	}
	require.Greater(t, expected(), 4*len(hashes))

	names := rangePartitions(k)
	left, err := analyzeRange(db, names)
	require.NoError(t, err)
	require.Len(t, left, len(names))
	require.LessOrEqual(t, expected(), len(hashes))
	analyzed, now := pages()
	require.Equal(t, now, analyzed)

	// grown by less than twice: no analyze
	insert(12001, 13000)
	_, err = analyzeRange(db, names)
	require.NoError(t, err)
	again, now := pages()
	require.Equal(t, analyzed, again)
	require.Less(t, analyzed, now)

	// twice the pages: analyzed again, and settled from rangeSettledPages
	insert(13001, 30000)
	saved := rangeSettledPages
	t.Cleanup(func() { rangeSettledPages = saved })
	rangeSettledPages = 2 * analyzed
	left, err = analyzeRange(db, names)
	require.NoError(t, err)
	again, now = pages()
	require.Equal(t, now, again)
	require.False(t, slices.Contains(left, name))
	require.Len(t, left, len(names)-1)
}
