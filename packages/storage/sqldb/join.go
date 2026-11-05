/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"fmt"
	"slices"

	"github.com/IBAX-io/go-ibax/packages/migration"
	"github.com/IBAX-io/go-ibax/packages/snapshot"
)

// PrepareJoined makes, on a node that imported the state of a snapshot (design 4.5), what of the
// retention the state needs out of itself, and returns the first block of the window: the
// partitioned node tables get the ranges of the state, those of rollback_tx, which leave by the
// same window, and the partitions the last boundary retired are empty tables of the retired
// schema, which the next boundary drops. The node has no rollback below the snapshot height
// (CheckRollbackTo), so nothing restores them.
func PrepareJoined(dbTx *DbTransaction) (int64, error) {
	ranges, err := attachedRanges(dbTx, "rollback_tx")
	if err != nil {
		return 0, err
	}
	if len(ranges) == 0 {
		return 0, fmt.Errorf("the state has no range of rollback_tx")
	}
	db := GetDB(dbTx)
	for _, t := range migration.PartitionedTables {
		if !snapshot.IsNodeTable(t.Name) {
			continue
		}
		have, err := attachedRanges(dbTx, t.Name)
		if err != nil {
			return 0, err
		}
		for _, r := range have {
			if !slices.Contains(ranges, r) {
				// a range of the node before the join, without rows: the node has block 1 alone
				if err := db.Exec(fmt.Sprintf(`DROP TABLE "%s"."%s"`, migration.PartsSchema,
					migration.PartitionName(t.Name, r))).Error; err != nil {
					return 0, err
				}
			}
		}
		for _, r := range ranges {
			if err := db.Exec(migration.CreatePartitionSQL(t.Name, r)).Error; err != nil {
				return 0, fmt.Errorf("creating the partition %d of %s: %w", r, t.Name, err)
			}
		}
	}
	retired, err := retiredBefore(dbTx, "<=", 1<<62)
	if err != nil {
		return 0, err
	}
	for _, r := range retired {
		if err := db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s"."%s" (LIKE "%s")`, migration.RetiredSchema,
			migration.PartitionName(r.TableName, r.K), r.TableName)).Error; err != nil {
			return 0, fmt.Errorf("retiring %s of %d: %w", r.TableName, r.K, err)
		}
	}
	return max(2, ranges[0]*migration.PartitionBlocks), nil
}

// InsertJoined stores blocks of the window of a node that joined, each with its certificate
func InsertJoined(dbTx *DbTransaction, blocks []BlockChain, certs []BftCertificate) error {
	if len(blocks) == 0 {
		return nil
	}
	db := GetDB(dbTx)
	if err := db.CreateInBatches(blocks, 64).Error; err != nil {
		return err
	}
	return db.CreateInBatches(certs, 512).Error
}
