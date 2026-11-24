/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/jackc/pgx/v5/pgconn"
	log "github.com/sirupsen/logrus"
)

// The blocks of a chain of the BFT consensus are ordered before they are executed (F18 design,
// direction A): the validators agree on a block of transactions, and every node executes each
// block the chain committed, once, in the order of the heights. A transaction that fails its
// checks at the execution is discarded: it changes nothing, pays nothing, and its hash is spent.
// The execution of block h leaves its execution hash (ExecHash), which block h+BftExecDepth
// carries, and the validators set of height h+BftSetDelay.

// The reasons of a discard, which the results hash takes: the text of the error does not, as it
// may carry what differs between nodes
const (
	// discardCheck is a check before the body of the contract: the signer, its key, the
	// ecosystem, the model of a transfer, the funds of the fee
	discardCheck int32 = 1
	// discardFuel is the fuel of the transaction or of the block (transaction.ErrFuelLimit)
	discardFuel int32 = 2
)

// ordered reports whether the block is of an ordered chain, executed after its order
func (b *Block) ordered() bool {
	return b.Header.Version >= consts.BvOrdered
}

// discardOf is the reason of the discard of a transaction whose execution failed with err; ok is
// false for an error of the node rather than of the transaction and the state, which fails the
// execution of the block: the node executes it again, and records no discard another node would
// not record
func discardOf(err error) (reason int32, ok bool) {
	switch {
	case err == nil, errors.Is(err, transaction.ErrNetworkStopping), infrastructure(err):
		return 0, false
	case errors.Is(err, transaction.ErrFuelLimit):
		return discardFuel, true
	}
	return discardCheck, true
}

// infrastructure reports whether err is of the database or of the node rather than of the
// transaction: an error of PostgreSQL, of the connection or of the network, a canceled context
func infrastructure(err error) bool {
	var (
		pg  *pgconn.PgError
		net net.Error
	)
	return errors.As(err, &pg) || errors.As(err, &net) ||
		errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) || errors.Is(err, sql.ErrTxDone) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// discardable is the reason of the discard of a transaction whose execution failed with err, when
// the block is an ordered block the node executes after its commit
func (b *Block) discardable(err error) (int32, bool) {
	if !b.ordered() || b.GenBlock || b.IsGenesis() {
		return 0, false
	}
	return discardOf(err)
}

// maxDiscardText bounds the text of the error a discarded transaction logs
const maxDiscardText = 255

// discard records the transaction t, which the execution of the block discarded with err for the
// reason: a log of the transaction of status DISCARDED, without rollback records, outputs or fee
func (b *Block) discard(afters *types.AfterTxs, t *transaction.Transaction, err error, reason int32) {
	eco, contract := int64(1), ""
	if t.IsSmartContract() {
		s := t.SmartContract()
		if s.TxSmart != nil {
			eco = s.TxSmart.EcosystemID
		}
		if s.TxContract != nil {
			contract = s.TxContract.Name
		}
	}
	text := err.Error()
	if len(text) > maxDiscardText {
		text = text[:maxDiscardText-3] + "..."
	}
	afters.Txs = append(afters.Txs, &types.AfterTx{
		UsedTx: t.Hash(),
		Lts: &types.LogTransaction{
			Block:        b.Header.BlockId,
			Hash:         t.Hash(),
			Timestamp:    t.Timestamp(),
			Address:      t.KeyID(),
			EcosystemId:  eco,
			ContractName: contract,
			InvokeStatus: pbgo.TxInvokeStatusCode_DISCARDED,
		},
		UpdTxStatus: &pbgo.TxResult{
			Hash:    t.Hash(),
			BlockId: b.Header.BlockId,
			Code:    pbgo.TxInvokeStatusCode_DISCARDED,
			Result:  text,
		},
	})
	if b.discards == nil {
		b.discards = make(map[string]int32)
	}
	b.discards[string(t.Hash())] = reason
	metrics.TxResults.WithLabelValues(b.execKind(), "discarded").Inc()
}

// resultsHash is the hash of the results of the transactions of the block, in their order: of each
// its hash, its status and the reason of its discard
func (b *Block) resultsHash() []byte {
	var buf []byte
	if b.AfterTxs != nil {
		buf = make([]byte, 0, len(b.AfterTxs.Txs)*(len(b.Header.BlockHash)+8))
		for _, after := range b.AfterTxs.Txs {
			buf = binary.BigEndian.AppendUint32(buf, uint32(len(after.Lts.Hash)))
			buf = append(buf, after.Lts.Hash...)
			buf = binary.BigEndian.AppendUint32(buf, uint32(after.Lts.InvokeStatus))
			buf = binary.BigEndian.AppendUint32(buf, uint32(b.discards[string(after.Lts.Hash)]))
		}
	}
	return crypto.Hash(buf)
}

// execHash is the execution hash of the block: of its height, its rollbacks hash, the hash of the
// results of its transactions and the hash of the validators set it leaves
func (b *Block) execHash() []byte {
	buf := binary.BigEndian.AppendUint64(nil, uint64(b.Header.BlockId))
	for _, part := range [][]byte{b.rollbacksHash, b.resultsHash(), crypto.Hash(b.nextValidators)} {
		buf = binary.BigEndian.AppendUint32(buf, uint32(len(part)))
		buf = append(buf, part...)
	}
	return crypto.Hash(buf)
}

// leaveValidators takes, in the transaction of the block, the validators set the execution of the
// block leaves: of the height consts.BftSetDelay after it on an ordered chain, of the next height
// on another. Block 1 leaves the set of height 2, which holds up to the first set a block changes.
func (b *Block) leaveValidators(dbTx *sqldb.DbTransaction) error {
	set, err := smart.BftValidatorsOf(dbTx, b.setHeight())
	if err != nil {
		return err
	}
	b.nextValidators = set.Marshal()
	return nil
}

// setHeight is the height of the validators set the execution of the block leaves
func (b *Block) setHeight() int64 {
	switch {
	case b.IsGenesis():
		return 2
	case b.ordered():
		return b.Header.BlockId + consts.BftSetDelay
	}
	return b.Header.BlockId + 1
}

// finishOrdered ends, in the transaction of the block, the execution of a block of an ordered
// chain whose transactions ran: the parameters its transactions changed, the validators set it
// leaves and its execution hash
func (b *Block) finishOrdered(dbTx *sqldb.DbTransaction) error {
	if b.SysUpdate {
		b.SysUpdate = false
		if err := syspar.SysUpdate(dbTx); err != nil {
			return fmt.Errorf("updating syspar: %w", err)
		}
	}
	if err := b.leaveValidators(dbTx); err != nil {
		return err
	}
	if err := b.storeValidators(dbTx); err != nil {
		return err
	}
	return b.storeExecHash(dbTx)
}

// storeExecHash makes the execution hash of the block, which executed and took the validators set
// it leaves, and writes it in the transaction of the block
func (b *Block) storeExecHash(dbTx *sqldb.DbTransaction) error {
	if b.nextValidators == nil {
		if err := b.leaveValidators(dbTx); err != nil {
			return err
		}
	}
	b.executed = b.execHash()
	if err := (&sqldb.BftExecHash{Height: b.Header.BlockId, Hash: b.executed}).SaveIn(dbTx); err != nil {
		return fmt.Errorf("storing the execution hash: %w", err)
	}
	return nil
}

// writeExecuted writes, last in the transaction of the execution of a committed block, its
// rollbacks hash to block_chain and the block to info_block: info_block is the last block the node
// executed
func (b *Block) writeExecuted(dbTx *sqldb.DbTransaction) error {
	bc := &sqldb.BlockChain{}
	found, err := bc.GetIn(dbTx, b.Header.BlockId)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("block %d is not committed", b.Header.BlockId)
	}
	if err := bc.SetRollbacksHash(dbTx, b.rollbacksHash); err != nil {
		return err
	}
	return b.upsertInfoBlock(dbTx, bc)
}

// Execute executes a block of an ordered chain the chain committed, as every node does once, in
// the order of the heights, in one database transaction: its transactions, those that fail their
// checks discarded, the fees, the records, the validators set it leaves, its execution hash and
// info_block. The block before it is executed. The contracts of the virtual machine, the language
// sources and the platform parameters are as they were before it if it fails.
func (b *Block) Execute() error {
	if !b.ordered() || b.IsGenesis() {
		return fmt.Errorf("block %d of version %d is not executed after its order", b.Header.BlockId, b.Header.Version)
	}
	logger := b.GetLogger()
	b.beginProfile()
	defer func() { b.prof = nil }()
	mem := takeMemState()
	dbTx, err := sqldb.StartTransaction()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("starting db transaction")
		return err
	}
	fail := func(err error) error {
		dbTx.Rollback()
		b.Notifications = nil
		if errMem := mem.restore(); errMem != nil {
			return errors.Join(err, errMem)
		}
		return err
	}
	b.verifySignatures()
	if err := b.ProcessTxs(dbTx); err != nil {
		return fail(err)
	}
	start := time.Now()
	if err := b.finishOrdered(dbTx); err != nil {
		return fail(err)
	}
	if err := b.writeExecuted(dbTx); err != nil {
		return fail(err)
	}
	b.stage(metrics.StageInsert, start)
	start = time.Now()
	if err := dbTx.Commit(); err != nil {
		return fail(err)
	}
	states.afterCommit(b.Header.BlockId)
	sqldb.AnalyzeRange(b.Header.BlockId)
	b.stage(metrics.StageCommit, start)
	start = time.Now()
	for _, q := range b.Notifications {
		q.Send()
	}
	b.stage(metrics.StageNotify, start)
	b.endProfile(dbTx)
	return nil
}
