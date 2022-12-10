/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpserver

import (
	"bytes"
	"errors"
	"io"

	"gorm.io/gorm/clause"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/utils"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"

	log "github.com/sirupsen/logrus"
)

// Disseminator get the list of transactions which belong to the sender from 'disseminator' daemon
// do not load the blocks here because here could be the chain of blocks that are loaded for a long time
// download the transactions here, because they are small and definitely will be downloaded in 60 sec
func Disseminator(rw io.ReadWriter) error {
	r := &network.DisRequest{}
	if err := r.Read(rw); err != nil {
		return err
	}

	buf := bytes.NewBuffer(r.Data)

	/*
	 *  data structure
	 *  type - 1 byte. 0 - block, 1 - list of transactions
	 *  {if type==1}:
	 *  <any number of the next sets>
	 *   tx_hash - 32 bytes
	 * </>
	 * {if type==0}:
	 *  block_id - 3 bytes
	 *  hash - 32 bytes
	 * <any number of the next sets>
	 *   tx_hash - 32 bytes
	 * </>
	 * */

	// honor_node_id of the sender to know where to take a data when it will be downloaded by another daemon
	honorNodeID := converter.BinToDec(buf.Next(8))
	log.Debug("honorNodeID", honorNodeID)
	n, err := syspar.GetNodeByPosition(honorNodeID)
	if err != nil {
		log.WithError(err).Error("on getting node by position")
		return err
	}

	// get data type (0 - block and transactions, 1 - only transactions)
	newDataType := converter.BinToDec(buf.Next(1))

	log.Debug("newDataType", newDataType)
	if newDataType == 0 {
		banned := n != nil && node.GetNodesBanService().IsBanned(*n)
		if banned {
			buf.Next(3)
			buf.Next(consts.HashSize)
		} else {
			err := processBlock(buf, honorNodeID)
			if err != nil {
				log.WithError(err).Error("on process block")
				return err
			}
		}
	}

	// get unknown transactions from received packet
	needTx, err := getUnknownTransactions(buf)
	if err != nil {
		log.WithError(err).Error("on getting unknown txes")
		return err
	}

	// send the list of transactions which we want to get
	err = (&network.DisHashResponse{Data: needTx}).Write(rw)
	if err != nil {
		log.WithError(err).Error("on sending neeeded tx list")
		return err
	}

	if len(needTx) == 0 {
		return nil
	}

	// get this new transactions
	txBodies, err := resieveTxBodies(rw)
	if err != nil {
		log.WithError(err).Error("on reading needed txes from disseminator")
		return err
	}

	// and save them
	return saveNewTransactions(txBodies)
}

func resieveTxBodies(con io.Reader) ([]byte, error) {
	sizeBuf := make([]byte, 4)
	if _, err := io.ReadFull(con, sizeBuf); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on getting size of tx bodies")
		return nil, err
	}

	size := converter.BinToDec(sizeBuf)
	txBodies := make([]byte, size)
	if _, err := io.ReadFull(con, txBodies); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("on getting tx bodies")
		return nil, err
	}

	return txBodies, nil
}

func processBlock(buf *bytes.Buffer, honorNodeID int64) error {
	infoBlock := &sqldb.InfoBlock{}
	found, err := infoBlock.Get()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting cur block ID")
		return utils.ErrInfo(err)
	}
	if !found {
		log.WithFields(log.Fields{"type": consts.NotFound}).Error("cant find info block")
		return errors.New("can't find info block")
	}

	// get block ID
	newBlockID := converter.BinToDec(buf.Next(3))
	log.WithFields(log.Fields{"new_block_id": newBlockID}).Debug("Generated new block id")

	// get block hash
	blockHash := buf.Next(consts.HashSize)
	log.Debug("blockHash %x", blockHash)

	qb := &sqldb.QueueBlock{}
	found, err = qb.GetQueueBlockByHash(blockHash)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting QueueBlock")
		return utils.ErrInfo(err)
	}
	// we accept only new blocks
	if !found && newBlockID >= infoBlock.BlockID {
		queueBlock := &sqldb.QueueBlock{Hash: blockHash, HonorNodeID: honorNodeID, BlockID: newBlockID}
		err = queueBlock.Create()
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Creating QueueBlock")
			return nil
		}
	}

	return nil
}

func getUnknownTransactions(buf *bytes.Buffer) ([]byte, error) {
	hashes, err := readHashes(buf)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ProtocolError, "error": err}).Error("on reading hashes")
		return nil, err
	}

	var needTx []byte
	// TODO: remove cycle, select miltiple txes throw in(?)
	for _, hash := range hashes {
		// check if we have such a transaction
		// check log_transaction
		exists, err := sqldb.GetLogTransactionsCount(hash)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err, "txHash": hash}).Error("Getting log tx count")
			return nil, utils.ErrInfo(err)
		}
		if exists > 0 {
			log.WithFields(log.Fields{"txHash": hash, "type": consts.DuplicateObject}).Warning("tx with this hash already exists in log_tx")
			continue
		}

		exists, err = sqldb.GetTransactionsCount(hash)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err, "txHash": hash}).Error("Getting tx count")
			return nil, utils.ErrInfo(err)
		}
		if exists > 0 {
			log.WithFields(log.Fields{"txHash": hash, "type": consts.DuplicateObject}).Warning("tx with this hash already exists in tx")
			continue
		}

		// check transaction queue
		exists, err = sqldb.GetQueuedTransactionsCount(hash)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting queue_tx count")
			return nil, utils.ErrInfo(err)
		}
		if exists > 0 {
			log.WithFields(log.Fields{"txHash": hash, "type": consts.DuplicateObject}).Warning("tx with this hash already exists in queue_tx")
			continue
		}
		needTx = append(needTx, hash...)
	}

	return needTx, nil
}

func readHashes(buf *bytes.Buffer) ([][]byte, error) {
	if buf.Len()%consts.HashSize != 0 {
		log.WithFields(log.Fields{"hashes_slice_size": buf.Len(), "tx_size": consts.HashSize, "type": consts.ProtocolError}).Error("incorrect hashes length")
		return nil, errors.New("wrong transactions hashes size")
	}

	hashes := make([][]byte, 0, buf.Len()/consts.HashSize)

	for buf.Len() > 0 {
		hashes = append(hashes, buf.Next(consts.HashSize))
	}

	return hashes, nil
}

func saveNewTransactions(binaryTxs []byte) error {
	var queueTxs []*sqldb.QueueTx
	log.WithFields(log.Fields{"binaryTxs": binaryTxs}).Debug("trying to save binary txs")

	for len(binaryTxs) > 0 {
		txSize, err := converter.DecodeLength(&binaryTxs)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.ProtocolError, "err": err}).Error("decoding binary txs length")
			return err
		}
		if int64(len(binaryTxs)) < txSize {
			log.WithFields(log.Fields{"type": consts.ProtocolError, "size": txSize, "len": len(binaryTxs)}).Error("incorrect binary txs len")
			return utils.ErrInfo(errors.New("bad transactions packet"))
		}

		txBinData := converter.BytesShift(&binaryTxs, txSize)
		if len(txBinData) == 0 {
			log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("binaryTxs is empty")
			return utils.ErrInfo(errors.New("len(txBinData) == 0"))
		}

		if int64(len(txBinData)) > syspar.GetMaxTxSize() {
			log.WithFields(log.Fields{"type": consts.ParameterExceeded, "len": len(txBinData), "size": syspar.GetMaxTxSize()}).Error("len of tx data exceeds max size")
			return utils.ErrInfo("len(txBinData) > max_tx_size")
		}

		rtx := transaction.Transaction{}
		if err = rtx.Unmarshall(bytes.NewBuffer(txBinData), true); err != nil {
			log.WithFields(log.Fields{"type": consts.UnmarshallingError, "error": err}).Error("unmarshalling transaction")
			return err
		}
		queueTxs = append(queueTxs, &sqldb.QueueTx{Hash: rtx.Hash(), Data: txBinData, Expedite: rtx.Expedite(), Time: rtx.Timestamp(), FromGate: 1})
	}
	if err := sqldb.GetDB(nil).Clauses(clause.OnConflict{DoNothing: true}).Create(&queueTxs).Error; err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("error creating QueueTx")
		return err
	}

	return nil
}
