/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"bytes"
	"errors"
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/common"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type maxBlockResult struct {
	MaxBlockID int64 `json:"max_block_id"`
}

func getMaxBlockHandler(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)

	block := &sqldb.BlockChain{}
	found, err := block.GetMaxBlock()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting max block")
		errorResponse(w, err)
		return
	}
	if !found {
		logger.WithFields(log.Fields{"type": consts.NotFound}).Debug("last block not found")
		errorResponse(w, errNotFound)
		return
	}

	jsonResponse(w, &maxBlockResult{block.ID})
}

type blockInfoResult struct {
	Hash          []byte `json:"hash"`
	EcosystemID   int64  `json:"ecosystem_id"`
	KeyID         int64  `json:"key_id"`
	Time          int64  `json:"time"`
	Tx            int32  `json:"tx_count"`
	RollbacksHash []byte `json:"rollbacks_hash"`
	NodePosition  int64  `json:"node_position"`
}

func getBlockInfoHandler(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	params := mux.Vars(r)

	blockID := converter.StrToInt64(params["id"])
	block := sqldb.BlockChain{}
	found, err := block.Get(blockID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting block")
		errorResponse(w, err)
		return
	}
	if !found {
		logger.WithFields(log.Fields{"type": consts.NotFound, "id": blockID}).Debug("block with id not found")
		errorResponse(w, errNotFound)
		return
	}

	jsonResponse(w, &blockInfoResult{
		Hash:          block.Hash,
		EcosystemID:   block.EcosystemID,
		KeyID:         block.KeyID,
		Time:          block.Time,
		Tx:            block.Tx,
		RollbacksHash: block.RollbacksHash,
		NodePosition:  block.NodePosition,
	})
}

type TxInfo struct {
	Hash         []byte         `json:"hash"`
	ContractName string         `json:"contract_name"`
	Params       map[string]any `json:"params"`
	KeyID        int64          `json:"key_id"`
}

type blocksTxInfoForm struct {
	BlockID int64 `schema:"block_id"`
	Count   int64 `schema:"count"`
}

func (f *blocksTxInfoForm) Validate(r *http.Request) error {
	if f.BlockID > 0 {
		f.BlockID--
	}
	if f.Count <= 0 {
		f.Count = defaultPaginatorLimit
	}

	if f.Count > maxPaginatorLimit {
		f.Count = maxPaginatorLimit
	}
	return nil
}

func getBlocksTxInfoHandler(w http.ResponseWriter, r *http.Request) {
	form := &blocksTxInfoForm{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	if form.BlockID < 0 || form.Count < 0 {
		err := errors.New("parameter is invalid")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	logger := getLogger(r)

	blocks, err := sqldb.GetBlockchain(form.BlockID, form.BlockID+form.Count, sqldb.OrderASC)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on getting blocks range")
		errorResponse(w, err)
		return
	}

	if len(blocks) == 0 {
		errorResponse(w, errNotFound)
		return
	}

	result := map[int64][]TxInfo{}
	for _, blockModel := range blocks {
		blck, err := block.UnmarshallBlock(bytes.NewBuffer(blockModel.Data), false)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.UnmarshallingError, "error": err, "bolck_id": blockModel.ID}).Error("on unmarshalling block")
			errorResponse(w, err)
			return
		}

		txInfoCollection := make([]TxInfo, 0, len(blck.Transactions))
		for _, tx := range blck.Transactions {
			txInfo := TxInfo{
				Hash: tx.Hash(),
			}

			if tx.IsSmartContract() {
				if tx.SmartContract().TxContract != nil {
					txInfo.ContractName = tx.SmartContract().TxContract.Name
				}
				txInfo.Params = tx.SmartContract().TxData
			}

			if blck.IsGenesis() {
				txInfo.KeyID = blck.Header.KeyId
			} else {
				txInfo.KeyID = tx.KeyID()
			}

			txInfoCollection = append(txInfoCollection, txInfo)

			logger.WithFields(log.Fields{"block_id": blockModel.ID, "tx hash": txInfo.Hash, "contract_name": txInfo.ContractName, "key_id": txInfo.KeyID, "params": txInfoCollection}).Debug("BlockChain Transactions Information")
		}

		result[blockModel.ID] = txInfoCollection
	}

	jsonResponse(w, &result)
}

type TxDetailedInfo struct {
	Hash         []byte         `json:"hash"`
	ContractName string         `json:"contract_name"`
	Params       map[string]any `json:"params"`
	KeyID        int64          `json:"key_id"`
	Time         int64          `json:"time"`
	Type         byte           `json:"type"`
	Size         string         `json:"size"`
}

type BlockHeaderInfo struct {
	BlockID      int64  `json:"block_id"`
	Time         int64  `json:"time"`
	EcosystemID  int64  `json:"-"`
	KeyID        int64  `json:"key_id"`
	NodePosition int64  `json:"node_position"`
	Sign         []byte `json:"-"`
	Hash         []byte `json:"-"`
	Version      int    `json:"version"`
}

type BlockDetailedInfo struct {
	Header        BlockHeaderInfo  `json:"header"`
	Hash          []byte           `json:"hash"`
	EcosystemID   int64            `json:"-"`
	NodePosition  int64            `json:"node_position"`
	KeyID         int64            `json:"key_id"`
	Time          int64            `json:"time"`
	Tx            int32            `json:"tx_count"`
	Size          string           `json:"size"`
	RollbacksHash []byte           `json:"rollbacks_hash"`
	MerkleRoot    []byte           `json:"merkle_root"`
	BinData       []byte           `json:"bin_data"`
	SysUpdate     bool             `json:"-"`
	GenBlock      bool             `json:"-"`
	StopCount     int              `json:"stop_count"`
	Transactions  []TxDetailedInfo `json:"transactions"`
}

func getBlocksDetailedInfoHandler(w http.ResponseWriter, r *http.Request) {
	form := &blocksTxInfoForm{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	if form.BlockID < 0 || form.Count < 0 {
		err := errors.New("parameter is invalid")
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	logger := getLogger(r)

	blocks, err := sqldb.GetBlockchain(form.BlockID, form.BlockID+form.Count, sqldb.OrderASC)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on getting blocks range")
		errorResponse(w, err)
		return
	}

	if len(blocks) == 0 {
		errorResponse(w, errNotFound)
		return
	}

	result := map[int64]BlockDetailedInfo{}
	for _, blockModel := range blocks {
		blck, err := block.UnmarshallBlock(bytes.NewBuffer(blockModel.Data), false)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.UnmarshallingError, "error": err, "block_id": blockModel.ID}).Error("on unmarshalling block")
			errorResponse(w, err)
			return
		}

		txDetailedInfoCollection := make([]TxDetailedInfo, 0, len(blck.Transactions))
		for _, tx := range blck.Transactions {
			txDetailedInfo := TxDetailedInfo{
				Hash:  tx.Hash(),
				KeyID: tx.KeyID(),
				Time:  tx.Timestamp(),
				Type:  tx.Type(),
				Size:  common.StorageSize(len(tx.Payload())).TerminalString(),
			}

			if tx.IsSmartContract() {
				if tx.SmartContract().TxContract != nil {
					txDetailedInfo.ContractName = tx.SmartContract().TxContract.Name
				}
				txDetailedInfo.Params = tx.SmartContract().TxData
				if tx.Type() == types.TransferSelfTxType {
					txDetailedInfo.Params = make(map[string]any)
					txDetailedInfo.Params["TransferSelf"] = tx.SmartContract().TxSmart.TransferSelf
				}
				if tx.Type() == types.UtxoTxType {
					txDetailedInfo.Params = make(map[string]any)
					txDetailedInfo.Params["UTXO"] = tx.SmartContract().TxSmart.UTXO
				}
			}

			txDetailedInfoCollection = append(txDetailedInfoCollection, txDetailedInfo)

			logger.WithFields(log.Fields{"block_id": blockModel.ID, "tx hash": txDetailedInfo.Hash, "contract_name": txDetailedInfo.ContractName, "key_id": txDetailedInfo.KeyID, "time": txDetailedInfo.Time, "type": txDetailedInfo.Type, "params": txDetailedInfoCollection}).Debug("BlockChain Transactions Information")
		}

		header := BlockHeaderInfo{
			BlockID:      blck.Header.BlockId,
			Time:         blck.Header.Timestamp,
			EcosystemID:  blck.Header.EcosystemId,
			KeyID:        blck.Header.KeyId,
			NodePosition: blck.Header.NodePosition,
			Sign:         blck.Header.Sign,
			Hash:         blck.Header.BlockHash,
			Version:      int(blck.Header.Version),
		}

		bdi := BlockDetailedInfo{
			Header:        header,
			Hash:          blockModel.Hash,
			EcosystemID:   blockModel.EcosystemID,
			NodePosition:  blockModel.NodePosition,
			KeyID:         blockModel.KeyID,
			Time:          blockModel.Time,
			Tx:            blockModel.Tx,
			RollbacksHash: blockModel.RollbacksHash,
			MerkleRoot:    blck.MerkleRoot,
			BinData:       blck.BinData,
			Size:          common.StorageSize(len(blockModel.Data)).TerminalString(),
			SysUpdate:     blck.SysUpdate,
			GenBlock:      blck.GenBlock,
			Transactions:  txDetailedInfoCollection,
		}
		result[blockModel.ID] = bdi
	}

	jsonResponse(w, &result)
}
