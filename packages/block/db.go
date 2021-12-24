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

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/protocols"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
)

// upsertInfoBlock updates info_block table
func (b *Block) upsertInfoBlock(dbTransaction *sqldb.DbTransaction, block *sqldb.BlockChain) error {
	ib := &sqldb.InfoBlock{
		Hash:          block.Hash,
		BlockID:       block.ID,
		Time:          block.Time,
		EcosystemID:   block.EcosystemID,
		KeyID:         block.KeyID,
		NodePosition:  converter.Int64ToStr(block.NodePosition),
		RollbacksHash: block.RollbacksHash,
	}
	if block.ID == 1 {
		ib.CurrentVersion = fmt.Sprintf("%d", consts.BlockVersion)
		err := ib.Create(dbTransaction)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating info block")
			return fmt.Errorf("error insert into info_block %s", err)
		}
	} else {
		ib.Sent = 0
		if err := ib.Update(dbTransaction); err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating info block")
			return fmt.Errorf("error while updating info_block %s", err)
		}
	}

	return nil
}

func GetRollbacksHash(transaction *sqldb.DbTransaction, blockID int64) ([]byte, error) {
	r := &sqldb.RollbackTx{}
	list, err := r.GetBlockRollbackTransactions(transaction, blockID)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)

	for _, rtx := range list {
		if err = enc.Encode(&rtx); err != nil {
			return nil, err
		}
	}

	return crypto.Hash(buf.Bytes()), nil
}

// InsertIntoBlockchain inserts a block into the blockchain
func (b *Block) InsertIntoBlockchain(dbTx *sqldb.DbTransaction) error {
	blockID := b.Header.BlockID
	bl := &sqldb.BlockChain{}
	err := bl.DeleteById(dbTx, blockID)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting block by id")
		return err
	}
	rollbacksHash, err := GetRollbacksHash(dbTx, blockID)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("getting rollbacks hash")
		return err
	}

	blockchain := &sqldb.BlockChain{
		ID:            blockID,
		Hash:          crypto.DoubleHash([]byte(b.Header.ForSha(b.PrevHeader, b.MrklRoot))),
		Data:          b.BinData,
		EcosystemID:   b.Header.EcosystemID,
		KeyID:         b.Header.KeyID,
		NodePosition:  b.Header.NodePosition,
		Time:          b.Header.Time,
		RollbacksHash: rollbacksHash,
		Tx:            int32(len(b.Transactions)),
	}
	var validBlockTime bool
	if blockID > 1 {
		validBlockTime, err = protocols.NewBlockTimeCounter().BlockForTimeExists(time.Unix(blockchain.Time, 0), int(blockchain.NodePosition))
		if err != nil {
			log.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("block validation")
			return err
		}
		if validBlockTime {
			err = fmt.Errorf("invalid block time: %d", b.Header.Time)
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

// GetBlockDataFromBlockChain is retrieving block data from blockchain
func GetBlockDataFromBlockChain(blockID int64) (*types.BlockData, error) {
	BlockData := new(types.BlockData)
	block := &sqldb.BlockChain{}
	_, err := block.Get(blockID)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting block by ID")
		return BlockData, err
	}

	header, _, err := types.ParseBlockHeader(bytes.NewBuffer(block.Data), syspar.GetMaxBlockSize())
	if err != nil {
		return nil, err
	}

	BlockData = &header
	BlockData.Hash = block.Hash
	BlockData.RollbacksHash = block.RollbacksHash
	return BlockData, nil
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
	tx, ok := t.Inner.(*transaction.FirstBlockTransaction)
	if !ok {
		log.WithFields(log.Fields{"type": consts.ParserError}).Error("getting data of first block")
		return
	}
	data = &tx.Data
	syspar.SysUpdate(nil)
	return
}
