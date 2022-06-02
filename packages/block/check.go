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
			transaction.MarkTransactionBad(t.DbTransaction, t.Hash(), err.Error())
			delete(txHashes, hexHash)
			b.Transactions = append(b.Transactions[:i], b.Transactions[i+1:]...)
			return errors.Wrap(err, "check transaction")
		}
	}

	// hash compare could be failed in the case of fork
	_, err = b.CheckHash()
	if err != nil {
		transaction.CleanCache()
		return err
	}
	return nil
}

// CheckHash is checking hash
func (b *Block) CheckHash() (bool, error) {
	logger := b.GetLogger()
	if b.IsGenesis() {
		return true, nil
	}
	if conf.Config.IsSubNode() {
		return true, nil
	}
	// check block signature
	if b.PrevHeader == nil {
		return true, nil
	}
	nodePublicKey, err := syspar.GetNodePublicKeyByPosition(b.Header.NodePosition)
	if err != nil {
		return false, utils.ErrInfo(err)
	}
	if len(nodePublicKey) == 0 {
		logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node public key is empty")
		return false, utils.ErrInfo(fmt.Errorf("empty nodePublicKey"))
	}

	_, err = utils.CheckSign([][]byte{nodePublicKey}, []byte(b.ForSign()), b.Header.Sign, true)

	if err != nil {
		//logger.WithFields(log.Fields{"error": err, "type": consts.CryptoError}).Error("checking block header sign")
		return false, errors.Wrap(err, "checking block header sign")
		//return false, errors.Wrap(err, fmt.Sprintf("per_block_id: %d, per_block_hash: %x", b.PrevHeader.BlockId, b.PrevHeader.Hash))
	}
	return true, nil
}
