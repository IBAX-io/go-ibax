/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"fmt"
	"sync"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/migration"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// The planner gives a condition on a column without statistics a fixed selectivity: the hashes of
// a block match, by its estimate, every row of a partition never analyzed, so that each lookup in
// log_transactions reads the partition of the active range whole instead of its index. A range
// starts empty at its boundary and fills a block a second; the autovacuum analyzes it a minute or
// more later, and every block meanwhile reads all the rows of the range so far, an order more than
// the rest of the block. The node analyzes the partitions of the range of each block it commits
// itself, once each holds rows: the statistics are no state of the chain, so it does so apart
// from the blocks. The analyze invalidates the plans the connections keep of the lookups.

// rangeStats are the partitions of range k the node has yet to analyze
type rangeStats struct {
	mu      sync.Mutex
	k       int64
	pending []string
	running bool
}

var activeRange = rangeStats{k: -1}

// AnalyzeRange analyzes, after the commit of the block height and without waiting, the partitions
// of its range that hold rows and have no statistics
func AnalyzeRange(height int64) {
	s := &activeRange
	k := height / migration.PartitionBlocks
	s.mu.Lock()
	defer s.mu.Unlock()
	if k != s.k {
		s.k, s.pending = k, rangePartitions(k)
	}
	if len(s.pending) == 0 || s.running {
		return
	}
	s.running = true
	pending := s.pending
	go func() {
		left, err := analyzeRange(DBConn, pending)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err, "range": k}).Warn("analyzing the partitions of the range")
		}
		s.mu.Lock()
		if s.k == k {
			s.pending = left
		}
		s.running = false
		s.mu.Unlock()
	}()
}

// rangePartitions are the partitions of range k of the partitioned tables
func rangePartitions(k int64) []string {
	names := make([]string, len(migration.PartitionedTables))
	for i, t := range migration.PartitionedTables {
		names[i] = migration.PartitionName(t.Name, k)
	}
	return names
}

// analyzeRange analyzes those of the partitions names that hold rows and have no statistics, and
// returns those still without; on an error, those it has not come to
func analyzeRange(db *gorm.DB, names []string) ([]string, error) {
	var analyzed []string
	if err := db.Raw(`SELECT DISTINCT "tablename"::text FROM "pg_stats" WHERE "schemaname" = ? AND "tablename"::text = ANY(?::text[])`,
		migration.PartsSchema, column(names, func(n string) string { return n })).Scan(&analyzed).Error; err != nil {
		return names, fmt.Errorf("reading the statistics of the partitions: %w", err)
	}
	done := make(map[string]bool, len(analyzed))
	for _, name := range analyzed {
		done[name] = true
	}
	var left []string
	for i, name := range names {
		if done[name] {
			continue
		}
		table := fmt.Sprintf(`"%s"."%s"`, migration.PartsSchema, name)
		var rows []bool
		if err := db.Raw(`SELECT true FROM ` + table + ` LIMIT 1`).Scan(&rows).Error; err != nil {
			return append(left, names[i:]...), fmt.Errorf("reading %s: %w", table, err)
		}
		if len(rows) == 0 {
			left = append(left, name)
			continue
		}
		if err := db.Exec(`ANALYZE ` + table).Error; err != nil {
			return append(left, names[i:]...), fmt.Errorf("analyzing %s: %w", table, err)
		}
	}
	return left, nil
}
