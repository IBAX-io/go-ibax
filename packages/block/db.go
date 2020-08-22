/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/crypto"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/protocols"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

// UpdBlockInfo updates info_block table
func UpdBlockInfo(dbTransaction *model.DbTransaction, block *Block) error {
	blockID := block.Header.BlockID
	// for the local tests
	forSha := block.Header.ForSha(block.PrevHeader, block.MrklRoot)

	hash := crypto.DoubleHash([]byte(forSha))

	block.Header.Hash = hash
	if block.IsGenesis() {
		ib := &model.InfoBlock{
			Hash:           hash,
			BlockID:        blockID,
			Time:           block.Header.Time,
			EcosystemID:    block.Header.EcosystemID,
			KeyID:          block.Header.KeyID,
			NodePosition:   converter.Int64ToStr(block.Header.NodePosition),
			CurrentVersion: fmt.Sprintf("%d", block.Header.Version),
		}
		err := ib.Create(dbTransaction)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating info block")
			return fmt.Errorf("error insert into info_block %s", err)
		}
	} else {
		ibUpdate := &model.InfoBlock{
			Hash:         hash,
			BlockID:      blockID,
			Time:         block.Header.Time,
			EcosystemID:  block.Header.EcosystemID,
			KeyID:        block.Header.KeyID,
			NodePosition: converter.Int64ToStr(block.Header.NodePosition),
			Sent:         0,
		}
		if err := ibUpdate.Update(dbTransaction); err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating info block")
			return fmt.Errorf("error while updating info_block: %s", err)
		}
	}

	return nil
}

func GetRollbacksHash(transaction *model.DbTransaction, blockID int64) ([]byte, error) {
	r := &model.RollbackTx{}
	list, err := r.GetBlockRollbackTransactions(transaction, blockID)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)

	}

	return crypto.Hash(buf.Bytes()), nil
}

// InsertIntoBlockchain inserts a block into the blockchain
func InsertIntoBlockchain(transaction *model.DbTransaction, block *Block) error {
	// for local tests
	blockID := block.Header.BlockID

	// record into the block chain
	bl := &model.Block{}
	err := bl.DeleteById(transaction, blockID)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting block by id")
		return err
	}
	rollbacksHash, err := GetRollbacksHash(transaction, blockID)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("getting rollbacks hash")
		return err
	}

	b := &model.Block{
		ID:            blockID,
		Hash:          block.Header.Hash,
		Data:          block.BinData,
		EcosystemID:   block.Header.EcosystemID,
		KeyID:         block.Header.KeyID,
		NodePosition:  block.Header.NodePosition,
		Time:          block.Header.Time,
		RollbacksHash: rollbacksHash,
		Tx:            int32(len(block.Transactions)),
	}
	validBlockTime := true
	if blockID > 1 {
		exists, err := protocols.NewBlockTimeCounter().BlockForTimeExists(time.Unix(b.Time, 0), int(b.NodePosition))
		if err != nil {
			log.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("block validation")
			return err
		}

		validBlockTime = !exists
	}
	if validBlockTime {
		if err = b.Create(transaction); err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating block")
			return err
		}
		if err = model.UpdRollbackHash(transaction, rollbacksHash); err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating info block")
			return err
		}
	} else {
		err := fmt.Errorf("invalid block time: %d", block.Header.Time)
		log.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("invalid block time")
		return err
	}

	return nil
}

// GetBlockDataFromBlockChain is retrieving block data from blockchain
func GetBlockDataFromBlockChain(blockID int64) (*utils.BlockData, error) {
	BlockData := new(utils.BlockData)
	block := &model.Block{}
	_, err := block.Get(blockID)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting block by ID")
		return BlockData, err
	}

	header, _, err := utils.ParseBlockHeader(bytes.NewBuffer(block.Data))
	if err != nil {
		return nil, err
	}

	BlockData = &header
	BlockData.Hash = block.Hash
	BlockData.RollbacksHash = block.RollbacksHash
	return BlockData, nil
}

// GetDataFromFirstBlock returns data of first block
func GetDataFromFirstBlock() (data *consts.FirstBlock, ok bool) {
	block := &model.Block{}
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
	data, ok = t.TxPtr.(*consts.FirstBlock)
	if !ok {
		log.WithFields(log.Fields{"type": consts.ParserError}).Error("getting data of first block")
		return
	}
	syspar.SysUpdate(nil)
	return
}
