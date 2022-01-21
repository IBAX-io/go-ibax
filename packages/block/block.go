/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/common/random"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/notificator"
	"github.com/IBAX-io/go-ibax/packages/protocols"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var (
	ErrIncorrectRollbackHash = errors.New("Rollback hash doesn't match")
	ErrEmptyBlock            = errors.New("Block doesn't contain transactions")
	ErrIncorrectBlockTime    = utils.WithBan(errors.New("Incorrect block time"))
)

// Block is storing block data
type Block struct {
	Header            types.BlockData
	PrevHeader        *types.BlockData
	PrevRollbacksHash []byte
	MrklRoot          []byte
	BinData           []byte
	Transactions      []*transaction.Transaction
	SysUpdate         bool
	GenBlock          bool // it equals true when we are generating a new block
	Notifications     []types.Notifications
}

func (b *Block) String() string {
	return fmt.Sprintf("header: %s, prevHeader: %s", b.Header, b.PrevHeader)
}

// GetLogger is returns logger
func (b *Block) GetLogger() *log.Entry {
	return log.WithFields(log.Fields{"block_id": b.Header.BlockID, "block_time": b.Header.Time, "block_wallet_id": b.Header.KeyID,
		"block_state_id": b.Header.EcosystemID, "block_hash": b.Header.Hash, "block_version": b.Header.Version})
}
func (b *Block) IsGenesis() bool {
	return b.Header.BlockID == 1
}

// PlaySafe is inserting block safely
func (b *Block) PlaySafe() error {
	logger := b.GetLogger()
	dbTransaction, err := sqldb.StartTransaction()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("starting db transaction")
		return err
	}

	inputTx := b.Transactions[:]
	err = b.Play(dbTransaction)
	if err != nil {
		dbTransaction.Rollback()
		if b.GenBlock && len(b.Transactions) == 0 {
			if err == transaction.ErrLimitStop {
				err = script.ErrVMTimeLimit
			}
			if inputTx[0].IsSmartContract() {
				transaction.BadTxForBan(inputTx[0].TxKeyID())
			}
			if err := transaction.MarkTransactionBad(dbTransaction, inputTx[0].TxHash(), err.Error()); err != nil {
				return err
			}
		}
		return err
	}

	if b.GenBlock {
		if len(b.Transactions) == 0 {
			dbTransaction.Commit()
			return ErrEmptyBlock
		} else if len(inputTx) != len(b.Transactions) {
			if err = b.repeatMarshallBlock(); err != nil {
				dbTransaction.Rollback()
				return err
			}
		}
	}

	if err := b.InsertIntoBlockchain(dbTransaction); err != nil {
		dbTransaction.Rollback()
		return err
	}

	err = dbTransaction.Commit()
	if err != nil {
		return err
	}

	for _, q := range b.Notifications {
		q.Send()
	}
	return nil
}

func (b *Block) repeatMarshallBlock() error {
	trData := make([][]byte, 0, len(b.Transactions))
	for _, tr := range b.Transactions {
		trData = append(trData, tr.FullData)
	}
	NodePrivateKey, _ := utils.GetNodeKeys()
	if len(NodePrivateKey) < 1 {
		err := errors.New(`empty private node key`)
		log.WithFields(log.Fields{"type": consts.NodePrivateKeyFilename, "error": err}).Error("reading node private key")
		return err
	}

	newBlockData, err := MarshallBlock(&b.Header, trData, b.PrevHeader, NodePrivateKey)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("marshalling new block")
		return err
	}

	nb, err := UnmarshallBlock(bytes.NewBuffer(newBlockData), true)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("parsing new block")
		return err
	}
	b.BinData = newBlockData
	b.Transactions = nb.Transactions
	b.MrklRoot = nb.MrklRoot
	b.SysUpdate = nb.SysUpdate
	return nil
}

func (b *Block) readPreviousBlockFromBlockchainTable() error {
	if b.IsGenesis() {
		b.PrevHeader = &types.BlockData{}
		return nil
	}

	var err error
	b.PrevHeader, err = GetBlockDataFromBlockChain(b.Header.BlockID - 1)
	if err != nil {
		return errors.Wrapf(err, "Can't get block %d", b.Header.BlockID-1)
	}
	return nil
}

func (b *Block) limitMode() transaction.LimitMode {
	if b == nil {
		return transaction.GetLetPreprocess()
	}
	if b.GenBlock {
		return transaction.GetLetGenBlock()
	}
	return transaction.GetLetParsing()
}

func (b *Block) Play(dbTransaction *sqldb.DbTransaction) (batchErr error) {
	var (
		playTxs sqldb.AfterTxs
	)
	logger := b.GetLogger()
	limits := transaction.NewLimits(b.limitMode())
	rand := random.NewRand(b.Header.Time)
	processedTx := make([]*transaction.Transaction, 0, len(b.Transactions))
	defer func() {
		if b.GenBlock {
			b.Transactions = processedTx
		}
		if err := sqldb.AfterPlayTxs(dbTransaction, b.Header.BlockID, playTxs, logger); err != nil {
			batchErr = err
			return
		}
	}()

	for curTx, t := range b.Transactions {
		err := dbTransaction.Savepoint(consts.SetSavePointMarkBlock(curTx))
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": t.TxHash()}).Error("using savepoint")
			return err
		}

		t.Notifications = notificator.NewQueue()
		t.DbTransaction = dbTransaction
		t.TxCheckLimits = limits
		t.PreBlockData = b.PrevHeader
		t.GenBlock = b.GenBlock
		t.SqlDbSavePoint = curTx
		t.Rand = rand.BytesSeed(t.TxHash())
		err = t.Play()
		if err != nil {
			if err == transaction.ErrNetworkStopping {
				// Set the node in a pause state
				node.PauseNodeActivity(node.PauseTypeStopingNetwork)
				return err
			}
			errRoll := t.DbTransaction.RollbackSavepoint(consts.SetSavePointMarkBlock(curTx))
			if errRoll != nil {
				t.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": t.TxHash()}).Error("rolling back to previous savepoint")
				return errRoll
			}
			if b.GenBlock {
				if err == transaction.ErrLimitStop {
					if curTx == 0 {
						return err
					}
					break
				}
				if strings.Contains(err.Error(), script.ErrVMTimeLimit.Error()) {
					err = script.ErrVMTimeLimit
				}
			}
			if t.IsSmartContract() {
				transaction.BadTxForBan(t.TxKeyID())
			}
			_ = transaction.MarkTransactionBad(t.DbTransaction, t.TxHash(), err.Error())
			if t.SysUpdate {
				if err := syspar.SysUpdate(t.DbTransaction); err != nil {
					log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating syspar")
					return err
				}
				t.SysUpdate = false
			}
			if b.GenBlock {
				continue
			}

			return err
		}

		if t.SysUpdate {
			b.SysUpdate = true
			t.SysUpdate = false
		}
		if err := sqldb.SetTransactionStatusBlockMsg(t.DbTransaction, t.BlockData.BlockID, t.TxResult, t.TxHash()); err != nil {
			t.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": t.TxHash()}).Error("updating transaction status block id")
			return err
		}
		if t.Notifications.Size() > 0 {
			b.Notifications = append(b.Notifications, t.Notifications)
		}
		playTxs.UsedTx = append(playTxs.UsedTx, t.TxHash())
		playTxs.Lts = append(playTxs.Lts, &sqldb.LogTransaction{Block: b.Header.BlockID, Hash: t.TxHash()})
		playTxs.Rts = append(playTxs.Rts, t.RollBackTx...)
		processedTx = append(processedTx, t)
	}

	return nil
}

// Check is checking block
func (b *Block) Check() error {
	if b.IsGenesis() {
		return nil
	}
	logger := b.GetLogger()
	if b.PrevHeader == nil || b.PrevHeader.BlockID != b.Header.BlockID-1 {
		if err := b.readPreviousBlockFromBlockchainTable(); err != nil {
			logger.WithFields(log.Fields{"type": consts.InvalidObject}).Error("block id is larger then previous more than on 1")
			return err
		}
	}
	if b.Header.Time > time.Now().Unix() {
		logger.WithFields(log.Fields{"type": consts.ParameterExceeded}).Error("block time is larger than now")
		return ErrIncorrectBlockTime
	}

	// is this block too early? Allowable error = error_time
	if b.PrevHeader != nil {
		// skip time validation for first block
		exists, err := protocols.NewBlockTimeCounter().BlockForTimeExists(time.Unix(b.Header.Time, 0), int(b.Header.NodePosition))
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("calculating block time")
			return err
		}

		if exists {
			logger.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Warn("incorrect block time")
			return utils.WithBan(fmt.Errorf("%s %d", ErrIncorrectBlockTime, b.PrevHeader.Time))
		}
	}

	// check each transaction
	txCounter := make(map[int64]int)
	txHashes := make(map[string]struct{})
	for i, t := range b.Transactions {
		hexHash := string(converter.BinToHex(t.TxHash()))
		// check for duplicate transactions
		if _, ok := txHashes[hexHash]; ok {
			logger.WithFields(log.Fields{"tx_hash": hexHash, "type": consts.DuplicateObject}).Warning("duplicate transaction")
			return utils.ErrInfo(fmt.Errorf("duplicate transaction %s", hexHash))
		}
		txHashes[hexHash] = struct{}{}

		// check for max transaction per user in one block
		txCounter[t.TxKeyID()]++
		if txCounter[t.TxKeyID()] > syspar.GetMaxBlockUserTx() {
			return utils.WithBan(utils.ErrInfo(fmt.Errorf("max_block_user_transactions")))
		}

		err := t.Check(b.Header.Time)
		if err != nil {
			transaction.MarkTransactionBad(t.DbTransaction, t.TxHash(), err.Error())
			delete(txHashes, hexHash)
			b.Transactions = append(b.Transactions[:i], b.Transactions[i+1:]...)
			return errors.Wrap(err, "check transaction")
		}
	}

	// hash compare could be failed in the case of fork
	_, err := b.CheckHash()
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
	if b.PrevHeader != nil {
		nodePublicKey, err := syspar.GetNodePublicKeyByPosition(b.Header.NodePosition)
		if err != nil {
			return false, utils.ErrInfo(err)
		}
		if len(nodePublicKey) == 0 {
			logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node public key is empty")
			return false, utils.ErrInfo(fmt.Errorf("empty nodePublicKey"))
		}

		signSource := b.Header.ForSign(b.PrevHeader, b.MrklRoot)

		resultCheckSign, err := utils.CheckSign(
			[][]byte{nodePublicKey},
			[]byte(signSource),
			b.Header.Sign,
			true)

		if err != nil {
			if err == crypto.ErrIncorrectSign {
				if !bytes.Equal(b.PrevRollbacksHash, b.PrevHeader.RollbacksHash) {
					return false, ErrIncorrectRollbackHash
				}
			}
			logger.WithFields(log.Fields{"error": err, "type": consts.CryptoError}).Error("checking block header sign")
			return false, utils.ErrInfo(fmt.Errorf("err: %v / block.PrevHeader.BlockID: %d /  block.PrevHeader.Hash: %x / ", err, b.PrevHeader.BlockID, b.PrevHeader.Hash))
		}

		return resultCheckSign, nil
	}

	return true, nil
}

// InsertBlockWOForks is inserting blocks
func InsertBlockWOForks(data []byte, genBlock, firstBlock bool) error {
	block, err := ProcessBlockWherePrevFromBlockchainTable(data, !firstBlock)
	if err != nil {
		return err
	}
	block.GenBlock = genBlock
	if err := block.Check(); err != nil {
		return err
	}

	err = block.PlaySafe()
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{"block_id": block.Header.BlockID}).Debug("block was inserted successfully")
	return nil
}

var (
	ErrMaxBlockSize    = utils.WithBan(errors.New("Block size exceeds maximum limit"))
	ErrZeroBlockSize   = utils.WithBan(errors.New("Block size is zero"))
	ErrUnmarshallBlock = utils.WithBan(errors.New("Unmarshall block"))
)

// ProcessBlockWherePrevFromBlockchainTable is processing block with in table previous block
func ProcessBlockWherePrevFromBlockchainTable(data []byte, checkSize bool) (*Block, error) {
	if checkSize && int64(len(data)) > syspar.GetMaxBlockSize() {
		log.WithFields(log.Fields{"check_size": checkSize, "size": len(data), "max_size": syspar.GetMaxBlockSize(), "type": consts.ParameterExceeded}).Error("binary block size exceeds max block size")
		return nil, ErrMaxBlockSize
	}

	buf := bytes.NewBuffer(data)
	if buf.Len() == 0 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("buffer is empty")
		return nil, ErrZeroBlockSize
	}

	block, err := UnmarshallBlock(buf, true)
	if err != nil {
		return nil, errors.Wrap(ErrUnmarshallBlock, err.Error())
	}
	block.BinData = data

	if err := block.readPreviousBlockFromBlockchainTable(); err != nil {
		return nil, err
	}

	return block, nil
}
