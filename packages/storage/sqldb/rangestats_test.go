/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"strings"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/migration"
	"github.com/stretchr/testify/require"
)

// TestAnalyzeRange: the lookup of the hashes of a block reads a partition with rows and without
// statistics whole, and the analyzed one by its index; the empty partitions of the range wait
func TestAnalyzeRange(t *testing.T) {
	db := testDB(t)
	const k = 900
	for _, table := range migration.PartitionedTables {
		require.NoError(t, db.Exec(migration.CreatePartitionSQL(table.Name, k)).Error)
	}
	require.NoError(t, db.Exec(`INSERT INTO "log_transactions" ("hash", "block")
		SELECT sha256(int8send(i)), ? + i / 4 FROM generate_series(1, 6000) i`, k*migration.PartitionBlocks).Error)
	hashes := make([][]byte, 512)
	for i := range hashes {
		h := sha256.Sum256(binary.BigEndian.AppendUint64(nil, uint64(i*11+1)))
		hashes[i] = h[:]
	}
	name := migration.PartitionName("log_transactions", k)
	scan := func() string {
		var plan []string
		require.NoError(t, db.Raw(`EXPLAIN SELECT "hash" FROM "log_transactions" WHERE "hash" = ANY(?::bytea[])`,
			column(hashes, func(h []byte) []byte { return h })).Scan(&plan).Error)
		for _, line := range plan {
			if strings.Contains(line, " on "+name+" ") || strings.HasSuffix(line, " on "+name) {
				return strings.TrimSpace(line)
			}
		}
		t.Fatalf("no scan of %s in %q", name, plan)
		return ""
	}
	require.Contains(t, scan(), "Seq Scan")

	names := rangePartitions(k)
	left, err := analyzeRange(db, names)
	require.NoError(t, err)
	require.Len(t, left, len(names)-1)
	require.False(t, slices.Contains(left, name))
	require.NotContains(t, scan(), "Seq Scan")

	again, err := analyzeRange(db, names)
	require.NoError(t, err)
	require.Equal(t, left, again)
}
