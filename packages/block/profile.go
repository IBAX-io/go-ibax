/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
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
	if b.prof != nil {
		b.prof.stage(name, since)
		return
	}
	metrics.BlockStage.WithLabelValues(b.execKind(), name).Observe(metrics.Since(since))
}

// note records in the profile of the execution running a time that is not a stage
func (b *Block) note(name string, d time.Duration) {
	if b.prof != nil {
		b.prof.note(name, d)
	}
}

// stage records a stage of the execution of the profile that began at since
func (p *execProfile) stage(name string, since time.Time) {
	d := time.Since(since)
	p.note(name, d)
	metrics.BlockStage.WithLabelValues(p.kind, name).Observe(d.Seconds())
}

// note records in the profile a time that is not a stage
func (p *execProfile) note(name string, d time.Duration) {
	p.fields[name+"_ms"] = float64(d.Microseconds()) / 1000
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
	p.end(b.Header.BlockId, b.txCount(), dbTx.Statements())
}

// end records the total of the execution of the profile, of block blockID, its txs transactions
// and the statements it sent to the database, and logs the profile
func (p *execProfile) end(blockID int64, txs int, statements int64) {
	total := time.Since(p.start)
	metrics.BlockStage.WithLabelValues(p.kind, metrics.StageTotal).Observe(total.Seconds())
	metrics.BlockTxs.WithLabelValues(p.kind).Observe(float64(txs))
	metrics.BlockStatements.WithLabelValues(p.kind).Observe(float64(statements))
	p.fields["kind"] = p.kind
	p.fields["block_id"] = blockID
	p.fields["txs"] = txs
	p.fields["db_statements"] = statements
	p.fields["total_ms"] = float64(total.Microseconds()) / 1000
	// an execution past the time a proposer has for its block is logged at warning level
	level := log.InfoLevel
	if total > time.Duration(syspar.GetMaxBlockGenerationTime())*time.Millisecond {
		level = log.WarnLevel
	}
	log.WithFields(p.fields).Log(level, "block execution profile")
}
