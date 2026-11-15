/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/migration"
	log "github.com/sirupsen/logrus"
)

// freezing is held by the one FreezeClosed that runs
var freezing sync.Mutex

// FreezeClosed freezes, out of the transactions of the blocks, the range partitions of the active
// tables below the range of the last block that rows were inserted into since their last vacuum:
// the ranges the inserts left, whose partitions do not vacuum on inserts (migration.PartitionOptions).
// A range is frozen once and its pages are not written again. It runs after the commit of a
// boundary block and as the node starts; while one runs, another call returns at once, and the
// next boundary takes what it left.
func FreezeClosed() {
	if !freezing.TryLock() {
		return
	}
	defer freezing.Unlock()
	parts, err := closedToFreeze()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("listing the ranges to freeze")
		return
	}
	for _, part := range parts {
		if err := DBConn.Exec(fmt.Sprintf(`VACUUM (FREEZE) "%s"."%s"`, migration.PartsSchema, part)).Error; err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err, "partition": part}).Error("freezing a range")
		}
	}
}

// closedToFreeze are the range partitions of the active tables below the range of the last
// block with rows inserted since their last vacuum
func closedToFreeze() ([]string, error) {
	var last int64
	if err := DBConn.Raw(`SELECT coalesce(max("id"), 0) FROM "block_chain"`).Scan(&last).Error; err != nil {
		return nil, err
	}
	var names []string
	if err := DBConn.Raw(`SELECT relname FROM pg_stat_user_tables
		WHERE schemaname = ? AND n_ins_since_vacuum > 0 ORDER BY relname`, migration.PartsSchema).
		Scan(&names).Error; err != nil {
		return nil, err
	}
	return closedRanges(names, last/migration.PartitionBlocks), nil
}

// closedRanges are the partitions of the names that are ranges of the partitioned tables below
// the range active
func closedRanges(names []string, active int64) []string {
	var parts []string
	for _, name := range names {
		for _, t := range migration.PartitionedTables {
			prefix := t.Name + "_p"
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			if r, err := strconv.ParseInt(name[len(prefix):], 10, 64); err == nil && r < active {
				parts = append(parts, name)
			}
		}
	}
	return parts
}
