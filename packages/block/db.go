/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/metrics"
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
	start := time.Now()
	block, err := UnmarshallBlock(bytes.NewBuffer(data), true)
	if err != nil {
		return nil, errors.Wrap(types.ErrUnmarshallBlock, err.Error())
	}
	metrics.BlockStage.WithLabelValues(metrics.KindReceived, metrics.StageParse).Observe(metrics.Since(start))
	block.PrevHeader, err = GetBlockHeaderFromBlockChain(block.Header.BlockId - 1)
	if err != nil {
		return nil, err
	}
	if block.PrevHeader == nil {
		return nil, errors.New("block previous header nil")
	}
	return block, nil
}

// rollbackHashes are the hashes of the rollback records rts, taken in parallel before the records
// have their ids, which rollbacksHash takes
func rollbackHashes(rts []*sqldb.RollbackTx) ([]string, error) {
	hashes := make([]string, len(rts))
	workers := min(runtime.GOMAXPROCS(0), len(rts)/256+1)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := w; i < len(rts) && errs[w] == nil; i += workers {
				hashes[i], errs[w] = rollbackHash(rts[i])
			}
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return hashes, nil
}

// rollbacksHash is the hash of the rollback records and the outputs a block writes, taken from
// the hashes of its rollback records (rollbackHashes) and its outputs rather than read back from
// the tables
func rollbacksHash(blockID int64, hashes []string, outputs []sqldb.SpentInfo) ([]byte, error) {
	arr := make([]string, 0, len(hashes)+len(outputs))
	arr = append(arr, hashes...)
	for _, row := range outputs {
		if row.BlockId != blockID {
			continue
		}
		data, err := json.Marshal(row)
		if err != nil {
			return nil, err
		}
		arr = append(arr, crypto.HashHex(data))
	}
	sort.Strings(arr)
	marshal, err := json.Marshal(arr)
	if err != nil {
		return nil, err
	}
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
	rHash := b.rollbacksHash
	if err = b.validatorsHash(dbTx); err != nil {
		return err
	}
	if b.GenBlock {
		b.Header.RollbacksHash = rHash
		if err = b.repeatMarshallBlock(); err != nil {
			return err
		}
		// blockSize counted every transaction in, so this only fails if that count is wrong
		if int64(len(b.BinData)) > syspar.GetMaxBlockSize() {
			return types.ErrMaxBlockSize(syspar.GetMaxBlockSize(), len(b.BinData))
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
	if blockID > 1 && syspar.IsHonorNodeMode() && !syspar.IsBftConsensus() {
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

// afterRows are the records the transactions of a block write: the logs and the statuses of the
// transactions, and the rollback records
type afterRows struct {
	rts      []*sqldb.RollbackTx
	lts      []*sqldb.LogTransaction
	statuses []*sqldb.TransactionStatus
}

// newAfterRows are the records of the transactions txs and of the rollback records rts
func newAfterRows(txs []*types.AfterTx, rts []*types.RollbackTx) *afterRows {
	rows := &afterRows{
		rts:      make([]*sqldb.RollbackTx, len(rts)),
		lts:      make([]*sqldb.LogTransaction, len(txs)),
		statuses: make([]*sqldb.TransactionStatus, len(txs)),
	}
	for i, tx := range rts {
		rows.rts[i] = &sqldb.RollbackTx{
			BlockID:   tx.BlockId,
			NameTable: tx.NameTable,
			Data:      tx.Data,
			TableID:   tx.TableId,
			TxHash:    tx.TxHash,
			DataHash:  tx.DataHash,
		}
	}
	for i, tx := range txs {
		rows.lts[i] = &sqldb.LogTransaction{
			Block:        tx.Lts.Block,
			Hash:         tx.Lts.Hash,
			Timestamp:    tx.Lts.Timestamp,
			Address:      tx.Lts.Address,
			EcosystemID:  tx.Lts.EcosystemId,
			ContractName: tx.Lts.ContractName,
			Status:       int64(tx.Lts.InvokeStatus),
		}
		if u := tx.UpdTxStatus; u != nil {
			var penalty int64
			if u.Code == pbgo.TxInvokeStatusCode_PENALTY {
				penalty = 1
			}
			rows.statuses[i] = &sqldb.TransactionStatus{
				Hash:     u.Hash,
				Time:     tx.Lts.Timestamp,
				WalletID: tx.Lts.Address,
				BlockID:  tx.Lts.Block,
				Error:    u.Result,
				Penalty:  penalty,
			}
		}
	}
	return rows
}

// writeTxRows writes the logs, the rollback records and the statuses of rows, each in a statement
func writeTxRows(tx *gorm.DB, rows *afterRows) error {
	if err := sqldb.CreateLogTransactionBatches(tx, rows.lts); err != nil {
		return errors.Wrap(err, "batches insert log_transactions")
	}
	if err := sqldb.CreateBatchesRollbackTx(tx, rows.rts); err != nil {
		return errors.Wrap(err, "batches insert rollback tx")
	}
	if err := sqldb.UpsertBlockStatusBatches(tx, rows.statuses); err != nil {
		return errors.Wrap(err, "batches upsert transaction status")
	}
	return nil
}

// AfterPlayTxs writes the records of the transactions of the block the stream did not write
// (afterStream), the counts of the logs, the outputs and the rollbacks hash of the block
func (b *Block) AfterPlayTxs(dbTx *sqldb.DbTransaction, written afterWritten) error {
	after := b.AfterTxs
	rest := newAfterRows(after.Txs[written.txs:], after.Rts[written.rts:])
	lts := append(written.lts, rest.lts...)
	hashes, err := rollbackHashes(rest.rts)
	if err != nil {
		return errors.Wrap(err, "rollbacks hash")
	}
	err = sqldb.GetDB(dbTx).Transaction(func(tx *gorm.DB) error {
		if err := writeTxRows(tx, rest); err != nil {
			return err
		}
		if err := sqldb.CountLogTransactions(tx, lts); err != nil {
			return errors.Wrap(err, "count log_transactions")
		}
		if err := sqldb.CreateStopNetworkTxs(tx, b.stopNetworkHashes(lts), b.Header.BlockId); err != nil {
			return errors.Wrap(err, "insert stop_network_txs")
		}
		spentInfos := sqldb.GetAllOutputs(b.OutputsMap)
		rHash, err := rollbacksHash(b.Header.BlockId, append(written.hashes, hashes...), spentInfos)
		if err != nil {
			return errors.Wrap(err, "rollbacks hash")
		}
		b.rollbacksHash = rHash
		return sqldb.WriteBlockOutputs(tx, b.Header.BlockId, spentInfos)
	})
	if err != nil || !sqldb.IsRetentionBoundary(b.Header.BlockId) {
		return err
	}
	return errors.Wrap(sqldb.Retain(dbTx, b.Header.BlockId, b.Header.Timestamp, syspar.GetHistoryBlocks()), "retention")
}

// stopNetworkHashes are the hashes of the StopNetwork transactions of the block: they are not
// bound by MaxTxBack, so that they stay known after their log_transactions leave
func (b *Block) stopNetworkHashes(lts []*sqldb.LogTransaction) [][]byte {
	logged := make(map[string]bool, len(lts))
	for _, lt := range lts {
		logged[string(lt.Hash)] = true
	}
	var hashes [][]byte
	for _, t := range b.Transactions {
		if t.Type() == types.StopNetworkTxType && logged[string(t.Hash())] {
			hashes = append(hashes, t.Hash())
		}
	}
	return hashes
}
