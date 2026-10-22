/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"time"

	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

// execProfile is where the time of one execution of a block goes: each stage is a metric, and at
// the end the whole profile is one log line of the block
type execProfile struct {
	kind   string
	start  time.Time
	fields log.Fields
}

// execKind is the kind of the execution of the block
func (b *Block) execKind() string {
	switch {
	case b.trial:
		return metrics.KindTrial
	case b.GenBlock:
		return metrics.KindGenerate
	default:
		return metrics.KindApply
	}
}

// beginProfile starts the profile of an execution of the block; the kind is read now, so trial
// is set before
func (b *Block) beginProfile() {
	b.prof = &execProfile{kind: b.execKind(), start: time.Now(), fields: log.Fields{}}
}

// stage records a stage of the execution that began at since
func (b *Block) stage(name string, since time.Time) {
	kind := b.execKind()
	d := time.Since(since)
	if b.prof != nil {
		kind = b.prof.kind
		b.prof.fields[name+"_ms"] = float64(d.Microseconds()) / 1000
	}
	metrics.BlockStage.WithLabelValues(kind, name).Observe(d.Seconds())
}

// txCount is the number of the transactions of the block: of a generated block, those it took
func (b *Block) txCount() int {
	if b.GenBlock {
		return len(b.TxFullData)
	}
	return len(b.Transactions)
}

// endProfile records the total of an execution that succeeded, its transactions and the
// statements it sent to the database, and logs the profile
func (b *Block) endProfile(dbTx *sqldb.DbTransaction) {
	p := b.prof
	b.prof = nil
	if p == nil {
		return
	}
	total := time.Since(p.start)
	txs, statements := b.txCount(), dbTx.Statements()
	metrics.BlockStage.WithLabelValues(p.kind, metrics.StageTotal).Observe(total.Seconds())
	metrics.BlockTxs.WithLabelValues(p.kind).Observe(float64(txs))
	metrics.BlockStatements.WithLabelValues(p.kind).Observe(float64(statements))
	p.fields["kind"] = p.kind
	p.fields["block_id"] = b.Header.BlockId
	p.fields["txs"] = txs
	p.fields["db_statements"] = statements
	p.fields["total_ms"] = float64(total.Microseconds()) / 1000
	log.WithFields(p.fields).Info("block execution profile")
}
