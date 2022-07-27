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
	logger := b.GetLogger()
	if b.PrevHeader.BlockId != b.Header.BlockId-1 {
		var err error
		b.PrevHeader, err = GetBlockHeaderFromBlockChain(b.Header.BlockId - 1)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.InvalidObject}).Error("block id is larger then previous more than on 1")
			return err
		}
	}

	if b.Header.Timestamp > time.Now().Unix() {
		logger.WithFields(log.Fields{"type": consts.ParameterExceeded}).Error("block time is larger than now")
		return ErrIncorrectBlockTime
	}
	var (
		exists bool
		err    error
	)
	if syspar.IsHonorNodeMode() {
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
	// check each transaction
	txCounter := make(map[int64]int)
	txHashes := make(map[string]struct{})
	for i, t := range b.Transactions {
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

		err := t.Check(b.Header.Timestamp)
		if err != nil {
			transaction.MarkTransactionBad(t.Hash(), err.Error())
			delete(txHashes, hexHash)
			b.Transactions = append(b.Transactions[:i], b.Transactions[i+1:]...)
			return errors.Wrap(err, "check transaction")
		}
	}

	// hash compare could be failed in the case of fork
	err = b.CheckSign()
	if err != nil {
		transaction.CleanCache()
		return err
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
	_, err = utils.CheckSign([][]byte{nodePub}, []byte(b.ForSign()), b.Header.Sign, true)
	if err != nil {
		return errors.Wrap(err, "checking block header sign")
	}
	return nil
}
