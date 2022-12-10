/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/protocols"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ProcessBlockByBinData is processing block with in table previous block
func ProcessBlockByBinData(data []byte, checkSize bool) (*Block, error) {
	if checkSize && int64(len(data)) > syspar.GetMaxBlockSize() {
		log.WithFields(log.Fields{"check_size": checkSize, "size": len(data), "max_size": syspar.GetMaxBlockSize(), "type": consts.ParameterExceeded}).Error("binary block size exceeds max block size")
		return nil, types.ErrMaxBlockSize(syspar.GetMaxBlockSize(), len(data))
	}
	block, err := UnmarshallBlock(bytes.NewBuffer(data), true)
	if err != nil {
		return nil, errors.Wrap(types.ErrUnmarshallBlock, err.Error())
	}
	block.PrevHeader, err = GetBlockHeaderFromBlockChain(block.Header.BlockId - 1)
	if err != nil {
		return nil, err
	}
	if block.PrevHeader == nil {
		return nil, errors.New("block previous header nil")
	}
	return block, nil
}

func (b *Block) GetRollbacksHash(dbTx *sqldb.DbTransaction) ([]byte, error) {
	r := &sqldb.RollbackTx{}
	diff, err := r.GetRollbacksDiff(dbTx, b.Header.BlockId)
	if err != nil {
		return nil, err
	}
	return crypto.Hash(diff), nil
}

func GetRollbacksHashWithDiffArr(dbTx *sqldb.DbTransaction, bId int64) ([]byte, error) {
	rollbackTx := sqldb.RollbackTx{}
	rollbackTxs, err := rollbackTx.GetBlockRollbackTransactions(dbTx, bId)
	if err != nil {
		return nil, err
	}
	arr := make([]string, 0)
	for _, row := range rollbackTxs {
		data, err := json.Marshal(row)
		if err != nil {
			continue
		}
		arr = append(arr, crypto.HashHex(data))
	}
	spentInfos, err := sqldb.GetBlockOutputs(dbTx, bId)
	if err != nil {
		return nil, err
	}
	for _, row := range spentInfos {
		data, err := json.Marshal(row)
		if err != nil {
			continue
		}
		arr = append(arr, crypto.HashHex(data))
	}
	sort.Strings(arr)
	marshal, _ := json.Marshal(arr)
	return crypto.Hash(marshal), nil
}

// InsertIntoBlockchain inserts a block into the blockchain
func (b *Block) InsertIntoBlockchain(dbTx *sqldb.DbTransaction) error {
	blockID := b.Header.BlockId
	bl := &sqldb.BlockChain{}
	err := bl.DeleteById(dbTx, blockID)
	if err != nil {
		b.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting block by id")
		return err
	}
	var rHash []byte
	rHash, err = GetRollbacksHashWithDiffArr(dbTx, blockID)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("getting rollbacks hash")
		return err
	}
	if b.GenBlock {
		b.Header.RollbacksHash = rHash
		if err = b.repeatMarshallBlock(); err != nil {
			return err
		}
	}
	blockchain := &sqldb.BlockChain{
		ID:             blockID,
		Hash:           b.Header.BlockHash,
		Data:           b.BinData,
		EcosystemID:    b.Header.EcosystemId,
		KeyID:          b.Header.KeyId,
		NodePosition:   b.Header.NodePosition,
		Time:           b.Header.Timestamp,
		RollbacksHash:  rHash,
		Tx:             int32(len(b.TxFullData)),
		ConsensusMode:  b.Header.ConsensusMode,
		CandidateNodes: b.Header.CandidateNodes,
	}
	var validBlockTime bool
	if blockID > 1 && syspar.IsHonorNodeMode() {
		validBlockTime, err = protocols.NewBlockTimeCounter().BlockForTimeExists(time.Unix(blockchain.Time, 0), int(blockchain.NodePosition))
		if err != nil {
			log.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("block validation")
			return err
		}
		if validBlockTime {
			err = fmt.Errorf("invalid block time: %d", b.Header.Timestamp)
			log.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("invalid block time")
			return err
		}
	}

	if err = blockchain.Create(dbTx); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating block")
		return err
	}
	if err := b.upsertInfoBlock(dbTx, blockchain); err != nil {
		return err
	}
	if b.SysUpdate {
		b.SysUpdate = false
		if err := syspar.SysUpdate(dbTx); err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating syspar")
			return err
		}
	}
	return nil
}

// GetBlockHeaderFromBlockChain is retrieving block data from blockchain
func GetBlockHeaderFromBlockChain(blockID int64) (*types.BlockHeader, error) {
	if blockID < 1 {
		return &types.BlockHeader{}, nil
	}
	block := new(sqldb.BlockChain)
	if _, err := block.Get(blockID); err != nil {
		return nil, errors.Wrapf(err, "find block by ID %d", blockID)
	}

	header, err := types.ParseBlockHeader(bytes.NewBuffer(block.Data), syspar.GetMaxBlockSize())
	if err != nil {
		return nil, errors.Wrapf(err, "parse block header by ID %d", blockID)
	}
	header.BlockHash = block.Hash
	header.RollbacksHash = block.RollbacksHash
	return header, nil
}

// GetDataFromFirstBlock returns data of first block
func GetDataFromFirstBlock() (data *types.FirstBlock, ok bool) {
	block := &sqldb.BlockChain{}
	isFound, err := block.Get(1)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting record of first block")
		return
	}

	if !isFound {
		return
	}

	pb, err := UnmarshallBlock(bytes.NewBuffer(block.Data), true)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ParserError, "error": err}).Error("parsing data of first block")
		return
	}

	if len(pb.Transactions) == 0 {
		log.WithFields(log.Fields{"type": consts.ParserError}).Error("list of parsers is empty")
		return
	}

	t := pb.Transactions[0]
	tx, ok := t.Inner.(*transaction.FirstBlockParser)
	if !ok {
		log.WithFields(log.Fields{"type": consts.ParserError}).Error("getting data of first block")
		return
	}
	data = tx.Data
	syspar.SetFirstBlockTimestamp(time.UnixMilli(tx.Timestamp).Unix())
	syspar.SysUpdate(nil)
	return
}

// upsertInfoBlock updates info_block table
func (b *Block) upsertInfoBlock(dbTx *sqldb.DbTransaction, block *sqldb.BlockChain) error {
	ib := &sqldb.InfoBlock{
		Hash:           block.Hash,
		BlockID:        block.ID,
		Time:           block.Time,
		EcosystemID:    block.EcosystemID,
		KeyID:          block.KeyID,
		NodePosition:   converter.Int64ToStr(block.NodePosition),
		RollbacksHash:  block.RollbacksHash,
		ConsensusMode:  block.ConsensusMode,
		CandidateNodes: block.CandidateNodes,
	}
	if block.ID == 1 {
		ib.CurrentVersion = fmt.Sprintf("%d", consts.BlockVersion)
		err := ib.Create(dbTx)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating info block")
			return fmt.Errorf("error insert into info_block %s", err)
		}
	} else {
		ib.Sent = 0
		if err := ib.Update(dbTx); err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating info block")
			return fmt.Errorf("error while updating info_block %s", err)
		}
	}

	return nil
}

type AfterTxs struct {
	UsedTx      [][]byte
	Rts         []*sqldb.RollbackTx
	Lts         []*sqldb.LogTransaction
	UpdTxStatus []*pbgo.TxResult
}

func (b *Block) GenAfterTxs() *AfterTxs {
	after := b.AfterTxs
	playTx := &AfterTxs{
		Rts:         make([]*sqldb.RollbackTx, len(after.Rts)),
		Lts:         make([]*sqldb.LogTransaction, len(after.Txs)),
		UpdTxStatus: make([]*pbgo.TxResult, len(after.Txs)),
	}

	for i := 0; i < len(after.Rts); i++ {
		tx := after.Rts[i]
		rt := new(sqldb.RollbackTx)
		rt.BlockID = tx.BlockId
		rt.NameTable = tx.NameTable
		rt.Data = tx.Data
		rt.TableID = tx.TableId
		rt.TxHash = tx.TxHash
		rt.DataHash = tx.DataHash
		playTx.Rts[i] = rt
	}

	for i := 0; i < len(after.Txs); i++ {
		tx := after.Txs[i]
		playTx.UsedTx = append(playTx.UsedTx, tx.UsedTx)
		lt := new(sqldb.LogTransaction)
		lt.Block = tx.Lts.Block
		lt.Hash = tx.Lts.Hash
		//lt.TxData = tx.Lts.TxData
		lt.Timestamp = tx.Lts.Timestamp
		lt.Address = tx.Lts.Address
		lt.EcosystemID = tx.Lts.EcosystemId
		lt.ContractName = tx.Lts.ContractName
		lt.Status = int64(tx.Lts.InvokeStatus)
		playTx.Lts[i] = lt

		u := new(pbgo.TxResult)
		u = tx.UpdTxStatus
		playTx.UpdTxStatus[i] = u
	}
	return playTx
}

func (b *Block) AfterPlayTxs(dbTx *sqldb.DbTransaction) error {
	playTx := b.GenAfterTxs()
	return sqldb.GetDB(dbTx).Transaction(func(tx *gorm.DB) error {
		//if !b.GenBlock && !b.IsGenesis() && conf.Config.BlockSyncMethod.Method == types.BlockSyncMethod_SQLDML.String() {
		//	for i := 0; i < len(b.AfterTxs.TxBinLogSql); i++ {
		//		if err := tx.Exec(string(b.AfterTxs.TxBinLogSql[i])).Error; err != nil {
		//			return errors.Wrap(err, "batches exec sql for tx")
		//		}
		//	}
		//}
		if err := sqldb.DeleteTransactions(tx, playTx.UsedTx); err != nil {
			return errors.Wrap(err, "batches delete used transactions")
		}
		if err := sqldb.CreateLogTransactionBatches(tx, playTx.Lts); err != nil {
			return errors.Wrap(err, "batches insert log_transactions")
		}
		spentInfos := sqldb.GetAllOutputs(b.OutputsMap)
		if len(spentInfos) > 0 {
			if err := sqldb.CreateSpentInfoBatches(tx, spentInfos); err != nil {
				return errors.Wrap(err, "batches insert spent_info")
			}
		}

		if err := sqldb.CreateBatchesRollbackTx(tx, playTx.Rts); err != nil {
			return errors.Wrap(err, "batches insert rollback tx")
		}
		if err := sqldb.UpdateBlockMsgBatches(tx, b.Header.BlockId, playTx.UpdTxStatus); err != nil {
			return errors.Wrap(err, "batches update block msg transaction status")
		}
		return nil
	})
}
