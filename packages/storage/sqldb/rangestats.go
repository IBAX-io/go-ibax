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
// the rest of the block. Nor is one analyze enough: a connection keeps the plan of a lookup until
// the next analyze, and the plan of a small partition reads it whole, which is cheaper there.
// The node analyzes the partitions of the range of each block it commits itself, whenever one
// has grown to twice its pages at the last analyze, up to rangeSettledPages, from where the
// index is the cheaper plan and the autovacuum keeps the statistics. The statistics are no state
// of the chain, so the node does so apart from the blocks.

// rangeStats are the partitions of range k the node has yet to analyze
type rangeStats struct {
	mu      sync.Mutex
	k       int64
	pending []string
	running bool
}

var activeRange = rangeStats{k: -1}

// rangeSettledPages are the pages of a partition from which the node leaves its statistics to the
// autovacuum: 32 MB, some 250,000 rows of log_transactions
var rangeSettledPages int64 = 4096

// AnalyzeRange analyzes, after the commit of the block height and without waiting, the partitions
// of its range that have grown to twice their pages at the last analyze
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

// analyzeRange analyzes those of the partitions names with pages that have twice the pages of
// their last analyze, and returns those below rangeSettledPages; on an error, those it has not
// come to
func analyzeRange(db *gorm.DB, names []string) ([]string, error) {
	var parts []struct {
		Name     string
		Analyzed int64
		Pages    int64
	}
	if err := db.Raw(`SELECT c.relname::text AS "name", c.relpages AS "analyzed", pg_relation_size(c.oid) / current_setting('block_size')::bigint AS "pages"
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = ? AND c.relname::text = ANY(?::text[]) ORDER BY 1`,
		migration.PartsSchema, column(names, func(n string) string { return n })).Scan(&parts).Error; err != nil {
		return names, fmt.Errorf("reading the pages of the partitions: %w", err)
	}
	var left []string
	for i, p := range parts {
		if p.Pages > 0 && p.Pages >= 2*p.Analyzed {
			table := fmt.Sprintf(`"%s"."%s"`, migration.PartsSchema, p.Name)
			if err := db.Exec(`ANALYZE ` + table).Error; err != nil {
				for _, q := range parts[i:] {
					left = append(left, q.Name)
				}
				return left, fmt.Errorf("analyzing %s: %w", table, err)
			}
			p.Analyzed = p.Pages
		}
		if p.Analyzed < rangeSettledPages {
			left = append(left, p.Name)
		}
	}
	return left, nil
}
