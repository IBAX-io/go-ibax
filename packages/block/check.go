/*----------------------------------------------------------------
- Copyright (c) IBAX. All rights reserved.
- See LICENSE in the project root for license information.
---------------------------------------------------------------*/

package block

import (
	"bytes"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/protocols"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Check is checking block
func (b *Block) Check() error {
	// skip validation for first block
	if b.IsGenesis() {
		return nil
	}
	if err := b.checkHeader(); err != nil {
		return err
	}
	if err := newTxChecks().check(b, b.Transactions); err != nil {
		return err
	}
	// hash compare could be failed in the case of fork
	return b.CheckSign()
}

// checkHeader checks the header of the block against the chain
func (b *Block) checkHeader() error {
	logger := b.GetLogger()
	if b.PrevHeader.BlockId != b.Header.BlockId-1 {
		var err error
		b.PrevHeader, err = GetBlockHeaderFromBlockChain(b.Header.BlockId - 1)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.InvalidObject}).Error("block id is larger then previous more than on 1")
			return err
		}
	}

	bftChain := syspar.IsBftConsensus()
	if bftChain {
		// a validator checks the time against its clock with bft_max_clock_drift_ms (Validate);
		// the time slots of the honor nodes do not apply
		if b.Header.Timestamp <= b.PrevHeader.Timestamp {
			return fmt.Errorf("%w: block %d at %d, not after block %d at %d", ErrIncorrectBlockTime,
				b.Header.BlockId, b.Header.Timestamp, b.PrevHeader.BlockId, b.PrevHeader.Timestamp)
		}
	} else if b.Header.Timestamp > time.Now().Unix() {
		logger.WithFields(log.Fields{"type": consts.ParameterExceeded}).Error("block time is larger than now")
		return ErrIncorrectBlockTime
	}
	if err := checkConsensusMode(b.Header, syspar.IsHonorNodesOnly()); err != nil {
		logger.WithFields(log.Fields{"type": consts.BlockError, "consensus_mode": b.Header.ConsensusMode}).Error("a chain of honor nodes only takes no block of another consensus mode")
		return err
	}
	var (
		exists bool
		err    error
	)
	if syspar.IsHonorNodeMode() && !bftChain {
		// is this block too early? Allowable error = error_time
		exists, err = protocols.NewBlockTimeCounter().BlockForTimeExists(time.Unix(b.Header.Timestamp, 0), int(b.Header.NodePosition))
	}
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("calculating block time")
		return err
	}

	if exists {
		logger.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Warn("incorrect block time")
		return utils.WithBan(fmt.Errorf("%s %d", ErrIncorrectBlockTime, b.PrevHeader.Timestamp))
	}
	if !bytes.Equal(b.PrevRollbacksHash, b.PrevHeader.RollbacksHash) {
		return ErrIncorrectRollbackHash
	}
	return nil
}

// txChecks checks the transactions of a block, which it takes in parts in their order
type txChecks struct {
	counter map[int64]int
	hashes  map[string]struct{}
}

func newTxChecks() *txChecks {
	return &txChecks{counter: make(map[int64]int), hashes: make(map[string]struct{})}
}

// check checks the transactions txs of the block, which follow those it checked: no transaction
// twice, at most max_block_user_transactions of a key, and each valid at the time of the block.
// A transaction that is not valid it takes out of the block.
func (c *txChecks) check(b *Block, txs []*transaction.Transaction) error {
	logger := b.GetLogger()
	txCounter, txHashes := c.counter, c.hashes
	checked := transaction.CheckBatch(txs, b.Header.Timestamp)
	for i, t := range txs {
		hexHash := string(converter.BinToHex(t.Hash()))
		// check for duplicate transactions
		if _, ok := txHashes[hexHash]; ok {
			logger.WithFields(log.Fields{"tx_hash": hexHash, "type": consts.DuplicateObject}).Warning("duplicate transaction")
			return utils.ErrInfo(fmt.Errorf("duplicate transaction %s", hexHash))
		}
		txHashes[hexHash] = struct{}{}

		// check for max transaction per user in one block
		txCounter[t.KeyID()]++
		if txCounter[t.KeyID()] > syspar.GetMaxBlockUserTx() {
			return utils.WithBan(utils.ErrInfo(fmt.Errorf("max_block_user_transactions")))
		}

		if err := checked[i]; err != nil {
			if !b.trial {
				transaction.MarkTransactionBad(t.Hash(), err.Error())
			}
			delete(txHashes, hexHash)
			if at := len(b.Transactions) - len(txs) + i; at >= 0 && at < len(b.Transactions) && b.Transactions[at] == t {
				b.Transactions = append(b.Transactions[:at], b.Transactions[at+1:]...)
			}
			return errors.Wrap(err, "check transaction")
		}
	}
	return nil
}

// checkConsensusMode refuses, on a chain of honor nodes only, a block of another consensus mode
func checkConsensusMode(header *types.BlockHeader, honorNodesOnly bool) error {
	if honorNodesOnly && header.ConsensusMode != consts.HonorNodeMode {
		return utils.WithBan(fmt.Errorf("consensus mode %d of block %d: only the honor nodes generate the blocks of this chain", header.ConsensusMode, header.BlockId))
	}
	return nil
}

func (b *Block) CheckSign() error {
	if b.IsGenesis() || conf.Config.IsSubNode() || b.PrevHeader == nil {
		return nil
	}
	nodePub, err := syspar.GetNodePublicKeyByPosition(b.Header.NodePosition)
	if err != nil {
		return fmt.Errorf("%v: %w", fmt.Sprintf("get node public key by position '%d'", b.Header.NodePosition), err)
	}
	if len(nodePub) == 0 {
		return fmt.Errorf("empty nodePublicKey")
	}
	err = utils.CheckNodeSign(nodePub, []byte(b.ForSign()), b.Header.Sign)
	if err != nil {
		return errors.Wrap(err, "checking block header sign")
	}
	return nil
}
