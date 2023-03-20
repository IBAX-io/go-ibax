/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/common"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strconv"
	"strings"
)

type blockChainApi struct {
}

func NewBlockChainApi() *blockChainApi {
	return &blockChainApi{}
}

func (b *blockChainApi) MaxBlockId(ctx RequestContext) (*int64, *Error) {
	r := ctx.HTTPRequest()
	logger := getLogger(r)

	bk := &sqldb.BlockChain{}
	found, err := bk.GetMaxBlock()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting max block")
		return nil, DefaultError(err.Error())
	}
	if !found {
		logger.WithFields(log.Fields{"type": consts.NotFound}).Debug("last block not found")
		return nil, NotFoundError()
	}

	return &bk.ID, nil
}

type BlockInfoResult struct {
	Hash          string `json:"hash"`
	EcosystemID   int64  `json:"ecosystem_id"`
	KeyID         int64  `json:"key_id"`
	Time          int64  `json:"time"`
	Tx            int32  `json:"tx_count"`
	RollbacksHash string `json:"rollbacks_hash"`
	NodePosition  int64  `json:"node_position"`
	ConsensusMode int32  `json:"consensus_mode"`
}

func (b *blockChainApi) GetBlockInfo(blockID int64) (*BlockInfoResult, *Error) {
	bk := sqldb.BlockChain{}
	found, err := bk.Get(blockID)
	if err != nil {
		return nil, DefaultError(err.Error())
	}
	if !found {
		return nil, NotFoundError()
	}

	result := &BlockInfoResult{
		Hash:          hex.EncodeToString(bk.Hash),
		EcosystemID:   bk.EcosystemID,
		KeyID:         bk.KeyID,
		Time:          bk.Time,
		Tx:            bk.Tx,
		RollbacksHash: hex.EncodeToString(bk.RollbacksHash),
		NodePosition:  bk.NodePosition,
		ConsensusMode: bk.ConsensusMode,
	}

	return result, nil
}

func (b *blockChainApi) HonorNodesCount() (*int64, *Error) {
	count := syspar.GetNumberOfNodesFromDB(nil)
	return &count, nil
}

type AppParamsForm struct {
	ecosystemForm
	paramsForm
	paginatorForm
}

func (f *AppParamsForm) Validate(r *http.Request) error {
	if f == nil {
		return errors.New(paramsEmpty)
	}
	err := f.ecosystemForm.Validate(r)
	if err != nil {
		return err
	}
	return f.paginatorForm.Validate(r)
}

type AppParamsResult struct {
	App  int64         `json:"app_id"`
	List []ParamResult `json:"list"`
}

func (b *blockChainApi) AppParams(ctx RequestContext, auth Auth, appId int64, ecosystem *int64, names *string, offset, limit *int) (*AppParamsResult, *Error) {
	form := &AppParamsForm{
		ecosystemForm: ecosystemForm{
			Validator: auth.EcosystemGetter,
		},
	}
	if ecosystem != nil {
		form.EcosystemID = *ecosystem
	}
	if names != nil {
		form.AcceptNames(*names)
	}
	if offset != nil {
		form.Offset = *offset
	}
	if limit != nil {
		form.Limit = *limit
	}

	r := ctx.HTTPRequest()
	if err := parameterValidator(r, form); err != nil {
		return nil, DefaultError(err.Error())
	}
	logger := getLogger(r)

	ap := &sqldb.AppParam{}
	ap.SetTablePrefix(form.EcosystemPrefix)

	list, err := ap.GetAllAppParameters(appId, &form.Offset, &form.Limit, form.Names)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting all app parameters")
	}

	result := &AppParamsResult{
		App:  appId,
		List: make([]ParamResult, 0),
	}

	for _, item := range list {
		result.List = append(result.List, ParamResult{
			ID:         converter.Int64ToStr(item.ID),
			Name:       item.Name,
			Value:      item.Value,
			Conditions: item.Conditions,
		})
	}

	return result, nil
}

type AppContentResult struct {
	Snippets  []sqldb.Snippet  `json:"snippets"`
	Pages     []sqldb.Page     `json:"pages"`
	Contracts []sqldb.Contract `json:"contracts"`
}

func (b *blockChainApi) GetAppContent(ctx RequestContext, auth Auth, appId int64) (*AppContentResult, *Error) {
	form := &AppParamsForm{
		ecosystemForm: ecosystemForm{
			Validator: auth.EcosystemGetter,
		},
	}
	r := ctx.HTTPRequest()

	if err := parameterValidator(r, form); err != nil {
		return nil, ParseError(err.Error())
	}

	logger := getLogger(r)

	sni := &sqldb.Snippet{}
	p := &sqldb.Page{}
	c := &sqldb.Contract{}
	ecosystemID := converter.StrToInt64(form.EcosystemPrefix)

	snippets, err := sni.GetByApp(appId, ecosystemID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting block interfaces by appID")
		return nil, DefaultError(err.Error())
	}

	pages, err := p.GetByApp(appId, ecosystemID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting pages by appID")
		return nil, DefaultError(err.Error())
	}

	contracts, err := c.GetByApp(appId, ecosystemID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting pages by appID")
		return nil, DefaultError(err.Error())
	}

	return &AppContentResult{
		Snippets:  snippets,
		Pages:     pages,
		Contracts: contracts,
	}, nil
}

// History Returns the change record for the entry in the specified data table in the current ecosystem
func (b *blockChainApi) History(ctx RequestContext, auth Auth, tableName string, tableId uint64) (*HistoryResult, *Error) {
	r := ctx.HTTPRequest()
	logger := getLogger(r)
	client := getClient(r)

	if tableName == "" || tableId <= 0 {
		return nil, InvalidParamsError("invalid params")
	}

	table := client.Prefix() + "_" + tableName
	rollbackTx := &sqldb.RollbackTx{}
	txs, err := rollbackTx.GetRollbackTxsByTableIDAndTableName(strconv.FormatUint(tableId, 10), table, rollbackHistoryLimit)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("rollback history")
		return nil, DefaultError(err.Error())
	}
	if txs == nil || len(*txs) == 0 {
		return nil, NotFoundError()
	}
	rollbackList := make([]map[string]string, 0, len(*txs))
	for _, tx := range *txs {
		if tx.Data == "" {
			continue
		}
		rollback := map[string]string{}
		if err := json.Unmarshal([]byte(tx.Data), &rollback); err != nil {
			logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling rollbackTx.Data from JSON")
			return nil, DefaultError(err.Error())
		}
		rollbackList = append(rollbackList, rollback)
	}

	return &HistoryResult{rollbackList}, nil
}

type blocksTxInfoForm struct {
	BlockID int64 `json:"block_id"`
	Count   int64 `json:"count"`
}

func (f *blocksTxInfoForm) Validate(r *http.Request) error {
	if f.BlockID <= 0 {
		return errors.New(fmt.Sprintf(invalidParams, strconv.FormatInt(f.BlockID, 10)))
	}
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

type TxInfo struct {
	Hash         string         `json:"hash"`
	ContractName string         `json:"contract_name"`
	Params       map[string]any `json:"params"`
	KeyID        int64          `json:"key_id"`
}

func (b *blockChainApi) GetBlocksTxInfo(ctx RequestContext, blockId, count int64) (*map[int64][]TxInfo, *Error) {
	r := ctx.HTTPRequest()
	form := &blocksTxInfoForm{
		BlockID: blockId,
		Count:   count,
	}
	if err := parameterValidator(r, form); err != nil {
		return nil, InvalidParamsError(err.Error())
	}

	logger := getLogger(r)

	blocks, err := sqldb.GetBlockchain(form.BlockID, form.BlockID+form.Count, sqldb.OrderASC)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on getting blocks range")
		return nil, DefaultError(err.Error())
	}

	if len(blocks) == 0 {
		return nil, NotFoundError()
	}

	result := map[int64][]TxInfo{}
	for _, blockModel := range blocks {
		blck, err := block.UnmarshallBlock(bytes.NewBuffer(blockModel.Data), false)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.UnmarshallingError, "error": err, "bolck_id": blockModel.ID}).Error("on unmarshalling block")
			return nil, DefaultError(err.Error())
		}

		txInfoCollection := make([]TxInfo, 0, len(blck.Transactions))
		for _, tx := range blck.Transactions {
			txInfo := TxInfo{
				Hash: hex.EncodeToString(tx.Hash()),
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

	return &result, nil
}

type TxDetailedInfo struct {
	Hash         string         `json:"hash"`
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
	Hash         string `json:"-"`
	Version      int    `json:"version"`
}

type BlockDetailedInfo struct {
	Header        BlockHeaderInfo  `json:"header"`
	Hash          string           `json:"hash"`
	EcosystemID   int64            `json:"-"`
	NodePosition  int64            `json:"node_position"`
	KeyID         int64            `json:"key_id"`
	Time          int64            `json:"time"`
	Tx            int32            `json:"tx_count"`
	Size          string           `json:"size"`
	RollbacksHash string           `json:"rollbacks_hash"`
	MerkleRoot    string           `json:"merkle_root"`
	BinData       string           `json:"bin_data"`
	SysUpdate     bool             `json:"-"`
	GenBlock      bool             `json:"-"`
	StopCount     int              `json:"stop_count"`
	Transactions  []TxDetailedInfo `json:"transactions"`
}

func (b *blockChainApi) DetailedBlocks(ctx RequestContext, blockId, count int64) (*map[int64]BlockDetailedInfo, *Error) {
	r := ctx.HTTPRequest()

	form := &blocksTxInfoForm{
		BlockID: blockId,
		Count:   count,
	}
	if err := form.Validate(r); err != nil {
		return nil, InvalidParamsError(err.Error())
	}

	logger := getLogger(r)

	blocks, err := sqldb.GetBlockchain(form.BlockID, form.BlockID+form.Count, sqldb.OrderASC)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on getting blocks range")
		return nil, DefaultError(err.Error())
	}

	if len(blocks) == 0 {
		return nil, NotFoundError()
	}

	result := map[int64]BlockDetailedInfo{}
	for _, blockModel := range blocks {
		blck, err := block.UnmarshallBlock(bytes.NewBuffer(blockModel.Data), false)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.UnmarshallingError, "error": err, "block_id": blockModel.ID}).Error("on unmarshalling block")
			return nil, DefaultError(err.Error())
		}

		txDetailedInfoCollection := make([]TxDetailedInfo, 0, len(blck.Transactions))
		for _, tx := range blck.Transactions {
			txDetailedInfo := TxDetailedInfo{
				Hash:  hex.EncodeToString(tx.Hash()),
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

			logger.WithFields(log.Fields{"block_id": blockModel.ID, "tx hash": txDetailedInfo.Hash,
				"contract_name": txDetailedInfo.ContractName, "key_id": txDetailedInfo.KeyID,
				"time": txDetailedInfo.Time, "type": txDetailedInfo.Type,
				"params": txDetailedInfoCollection}).Debug("BlockChain Transactions Information")
		}

		header := BlockHeaderInfo{
			BlockID:      blck.Header.BlockId,
			Time:         blck.Header.Timestamp,
			EcosystemID:  blck.Header.EcosystemId,
			KeyID:        blck.Header.KeyId,
			NodePosition: blck.Header.NodePosition,
			Sign:         blck.Header.Sign,
			Hash:         hex.EncodeToString(blck.Header.BlockHash),
			Version:      int(blck.Header.Version),
		}

		bdi := BlockDetailedInfo{
			Header:        header,
			Hash:          hex.EncodeToString(blockModel.Hash),
			EcosystemID:   blockModel.EcosystemID,
			NodePosition:  blockModel.NodePosition,
			KeyID:         blockModel.KeyID,
			Time:          blockModel.Time,
			Tx:            blockModel.Tx,
			RollbacksHash: hex.EncodeToString(blockModel.RollbacksHash),
			MerkleRoot:    hex.EncodeToString(blck.MerkleRoot),
			BinData:       hex.EncodeToString(blck.BinData),
			Size:          common.StorageSize(len(blockModel.Data)).TerminalString(),
			SysUpdate:     blck.SysUpdate,
			GenBlock:      blck.GenBlock,
			Transactions:  txDetailedInfoCollection,
		}
		result[blockModel.ID] = bdi
	}

	return &result, nil
}

type BlockIdOrHash struct {
	Id     int64  `json:"id,omitempty"`
	Hash   string `json:"hash,omitempty"`
	isHash bool
}

func (bh *BlockIdOrHash) GetHash() ([]byte, bool) {
	if bh.Hash != "" {
		hash, err := hex.DecodeString(bh.Hash)
		if err != nil {
			return nil, false
		}
		return hash, true
	}
	return nil, false
}

func (bh *BlockIdOrHash) GetBlock() (int64, bool) {
	if bh.Id > 0 {
		return bh.Id, true
	}
	return 0, false
}

func (bh *BlockIdOrHash) Validate(r *http.Request) error {
	if bh == nil {
		return errors.New(paramsEmpty)
	}
	_, f1 := bh.GetHash()
	_, f2 := bh.GetBlock()
	if !f1 && !f2 {
		return errors.New(fmt.Sprintf(invalidParams, "block Id Or block Hash"))
	}
	if f1 {
		bh.isHash = f1
	}
	return nil
}

func (bh *BlockIdOrHash) UnmarshalJSON(data []byte) error {
	type rename BlockIdOrHash
	info := rename{}
	err := json.Unmarshal(data, &info)
	if err == nil {
		if info.Id != 0 && info.Hash != "" {
			return fmt.Errorf("block id or block hash must be only choose one")
		}
		bh.Id = info.Id
		bh.Hash = info.Hash
		return nil
	}
	var input string
	err = json.Unmarshal(data, &input)
	if err != nil {
		return err
	}
	if len(input) == 64 {
		bh.Hash = input
		return nil
	} else {
		if !smart.CheckNumberChars(input) {
			return errors.New("invalid block id or block hash")
		}

		blockNum, err := strconv.ParseInt(input, 10, 64)
		if err != nil {
			return err
		}
		bh.Id = blockNum
		return nil
	}
}

func (b *blockChainApi) DetailedBlock(ctx RequestContext, bh *BlockIdOrHash) (*BlockDetailedInfo, *Error) {
	r := ctx.HTTPRequest()

	err := parameterValidator(r, bh)
	if err != nil {
		return nil, InvalidParamsError(err.Error())
	}
	logger := getLogger(r)

	bk := &sqldb.BlockChain{}
	var f bool
	if bh.isHash {
		hash, _ := bh.GetHash()
		f, err = bk.GetByHash(hash)
	} else {
		f, err = bk.Get(bh.Id)
	}
	if err != nil {
		return nil, DefaultError(err.Error())
	}
	if !f {
		return nil, NotFoundError()
	}

	blck, err := block.UnmarshallBlock(bytes.NewBuffer(bk.Data), false)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.UnmarshallingError, "error": err, "block_id": bk.ID}).Error("on unmarshalling block")
		return nil, DefaultError(err.Error())
	}

	txDetailedInfoCollection := make([]TxDetailedInfo, 0, len(blck.Transactions))
	for _, tx := range blck.Transactions {
		txDetailedInfo := TxDetailedInfo{
			Hash:  hex.EncodeToString(tx.Hash()),
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

		logger.WithFields(log.Fields{"block_id": bk.ID, "tx hash": txDetailedInfo.Hash,
			"contract_name": txDetailedInfo.ContractName, "key_id": txDetailedInfo.KeyID,
			"time": txDetailedInfo.Time, "type": txDetailedInfo.Type,
			"params": txDetailedInfoCollection}).Debug("[GetBlock]BlockChain Transactions Information")
	}

	header := BlockHeaderInfo{
		BlockID:      blck.Header.BlockId,
		Time:         blck.Header.Timestamp,
		EcosystemID:  blck.Header.EcosystemId,
		KeyID:        blck.Header.KeyId,
		NodePosition: blck.Header.NodePosition,
		Sign:         blck.Header.Sign,
		Hash:         hex.EncodeToString(blck.Header.BlockHash),
		Version:      int(blck.Header.Version),
	}

	result := BlockDetailedInfo{
		Header:        header,
		Hash:          hex.EncodeToString(bk.Hash),
		EcosystemID:   bk.EcosystemID,
		NodePosition:  bk.NodePosition,
		KeyID:         bk.KeyID,
		Time:          bk.Time,
		Tx:            bk.Tx,
		RollbacksHash: hex.EncodeToString(bk.RollbacksHash),
		MerkleRoot:    hex.EncodeToString(blck.MerkleRoot),
		BinData:       hex.EncodeToString(blck.BinData),
		Size:          common.StorageSize(len(bk.Data)).TerminalString(),
		SysUpdate:     blck.SysUpdate,
		GenBlock:      blck.GenBlock,
		Transactions:  txDetailedInfoCollection,
	}

	return &result, nil
}

func (b *blockChainApi) GetTransactionCount(ctx RequestContext, bh *BlockIdOrHash) (*int32, *Error) {
	r := ctx.HTTPRequest()

	err := parameterValidator(r, bh)
	if err != nil {
		return nil, InvalidParamsError(err.Error())
	}

	bk := &sqldb.BlockChain{}
	var f bool
	if bh.isHash {
		hash, _ := bh.GetHash()
		f, err = bk.GetByHash(hash)
	} else {
		f, err = bk.Get(bh.Id)
	}
	if err != nil {
		return nil, DefaultError(err.Error())
	}
	if !f {
		return nil, NotFoundError()
	}

	return &bk.Tx, nil
}

func (b *blockChainApi) GetEcosystemParams(ctx RequestContext, auth Auth, ecosystem *int64, names *string, offset, limit *int) (*ParamsResult, *Error) {
	r := ctx.HTTPRequest()
	form := &AppParamsForm{
		ecosystemForm: ecosystemForm{
			Validator: auth.EcosystemGetter,
		}}
	if ecosystem != nil {
		form.EcosystemID = *ecosystem
	}
	if names != nil {
		form.AcceptNames(*names)
	}
	if limit != nil {
		form.Limit = *limit
	}
	if offset != nil {
		form.Offset = *offset
	}
	if err := parameterValidator(r, form); err != nil {
		return nil, DefaultError(err.Error())
	}

	logger := getLogger(r)

	sp := &sqldb.StateParameter{}
	sp.SetTablePrefix(form.EcosystemPrefix)
	list, err := sp.GetAllStateParameters(&form.Offset, &form.Limit, form.Names)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting all state parameters")
	}

	result := &ParamsResult{
		List: make([]ParamResult, 0),
	}

	for _, item := range list {
		result.List = append(result.List, ParamResult{
			ID:         converter.Int64ToStr(item.ID),
			Name:       item.Name,
			Value:      item.Value,
			Conditions: item.Conditions,
		})
	}

	return result, nil
}

type EcosystemInfo struct {
	Id           int64  `json:"id"`
	Name         string `json:"name"`
	Digits       int64  `json:"digits"`
	TokenSymbol  string `json:"token_symbol"`
	TokenName    string `json:"token_name"`
	TotalAmount  string `json:"total_amount"`
	IsWithdraw   bool   `json:"is_withdraw"`
	Withdraw     string `json:"withdraw"`
	IsEmission   bool   `json:"is_emission"`
	Emission     string `json:"emission"`
	Introduction string `json:"introduction"`
	Logo         int64  `json:"logo"`
	Creator      string `json:"creator"`
}

var totalSupplyToken = decimal.New(2100000000, int32(consts.MoneyDigits))

func (b *blockChainApi) EcosystemInfo(ctx RequestContext, ecosystemId int64) (*EcosystemInfo, *Error) {
	r := ctx.HTTPRequest()
	logger := getLogger(r)

	para := &sqldb.StateParameter{}
	para.SetTablePrefix(strconv.FormatInt(ecosystemId, 10))
	f, err := para.Get(nil, "founder_account")
	if err != nil {
		return nil, DefaultError(err.Error())
	}
	if !f {
		return nil, NotFoundError()
	}

	eco := &sqldb.Ecosystem{}
	found, err := eco.Get(nil, ecosystemId)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on getting ecosystem name")
		return nil, DefaultError(err.Error())
	}
	if !found {
		logger.WithFields(log.Fields{"type": consts.NotFound, "ecosystem_id": ecosystemId}).Debug("ecosystem by id not found")
		return nil, NotFoundError()
	}

	info := &EcosystemInfo{}
	keyId, err := strconv.ParseInt(para.Value, 10, 64)
	if err != nil {
		log.WithFields(log.Fields{"error": err, "creator:": para.Value}).Debug("get Ecosystem Creator Failed")
		return nil, DefaultError(err.Error())
	}
	type emsAmount struct {
		Val  decimal.Decimal `json:"val"`
		Time string          `json:"time"`
		Type string          `json:"type"`
	}
	var emissionAmount []emsAmount
	total := decimal.New(0, 0)
	withdraw := decimal.New(0, 0)
	emission := decimal.New(0, 0)
	if eco.EmissionAmount != "" {
		info := eco.EmissionAmount
		if err := json.Unmarshal([]byte(info), &emissionAmount); err != nil {
			return nil, DefaultError(err.Error())
		}
		for _, v := range emissionAmount {
			switch v.Type {
			case "issue":
				total = total.Add(v.Val)
			case "emission":
				emission = emission.Add(v.Val)
			case "burn":
				withdraw = withdraw.Add(v.Val)
			}
		}
	}
	if eco.Info != "" {
		minfo := make(map[string]any)
		err := json.Unmarshal([]byte(eco.Info), &minfo)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.UnmarshallingError, "error": err}).Error("Get Ecosystem Info Failed")
			return nil, DefaultError(err.Error())
		}
		logo, ok := minfo["logo"]
		if ok {
			logoId, err := strconv.ParseInt(strings.Replace(fmt.Sprint(logo), `'`, `''`, -1), 10, 64)
			if err != nil {
				return nil, DefaultError(err.Error())
			}
			info.Logo = logoId
		}
		for k, v := range minfo {
			switch k {
			case "description":
				info.Introduction = fmt.Sprint(v)
			}
		}
	}

	info.Creator = converter.AddressToString(keyId)
	info.Id = eco.ID
	info.Name = eco.Name
	info.TokenSymbol = eco.TokenSymbol
	info.TokenName = eco.TokenName
	info.Digits = eco.Digits
	if eco.TypeWithdraw == 1 {
		info.IsWithdraw = true
	}
	if eco.TypeEmission == 1 {
		info.IsEmission = true
	}
	info.Withdraw = withdraw.String()
	info.Emission = emission.String()
	if info.Id == 1 {
		info.TotalAmount = totalSupplyToken.String()
		return info, nil
	}
	info.TotalAmount = total.String()

	return info, nil
}

func (b *blockChainApi) SystemParams(ctx RequestContext, auth Auth, ecosystemId *int64, names *string, offset, limit *int) (*ParamsResult, *Error) {
	r := ctx.HTTPRequest()
	form := &AppParamsForm{
		ecosystemForm: ecosystemForm{
			Validator: auth.EcosystemGetter,
		},
	}
	if ecosystemId != nil {
		form.EcosystemID = *ecosystemId
	}
	if names != nil {
		form.AcceptNames(*names)
	}
	if offset != nil {
		form.Offset = *offset
	}
	if limit != nil {
		form.Limit = *limit
	}
	if err := parameterValidator(r, form); err != nil {
		return nil, DefaultError(err.Error())
	}

	logger := getLogger(r)

	list, err := sqldb.GetAllPlatformParameters(nil, &form.Offset, &form.Limit, form.Names)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting all platform parameters")
	}

	result := &ParamsResult{
		List: make([]ParamResult, 0),
	}

	for _, item := range list {
		result.List = append(result.List, ParamResult{
			ID:         converter.Int64ToStr(item.ID),
			Name:       item.Name,
			Value:      item.Value,
			Conditions: item.Conditions,
		})
	}

	if len(result.List) == 0 {
		return nil, NotFoundError()
	}

	return result, nil
}

type MemberInfo struct {
	ID         int64  `json:"id"`
	MemberName string `json:"member_name"`
	ImageID    *int64 `json:"image_id"`
	MemberInfo string `json:"member_info"`
}

func (b *blockChainApi) GetMember(ctx RequestContext, account string, ecosystemId int64) (*MemberInfo, *Error) {
	r := ctx.HTTPRequest()
	logger := getLogger(r)

	keyId := converter.AddressToID(account)
	if keyId == 0 {
		return nil, InvalidParamsError(fmt.Sprintf("account[%s] address invalid", account))
	}
	if ecosystemId <= 0 {
		return nil, InvalidParamsError(fmt.Sprintf("ecosystem id invalid"))
	}

	member := &sqldb.Member{}
	member.SetTablePrefix(converter.Int64ToStr(ecosystemId))

	_, err := member.Get(account)
	if err != nil {
		logger.WithFields(log.Fields{
			"type":      consts.DBError,
			"error":     err,
			"ecosystem": ecosystemId,
			"account":   account,
		}).Error("getting member")
		return nil, DefaultError(err.Error())
	}
	info := &MemberInfo{
		ID:         member.ID,
		MemberName: member.MemberName,
		ImageID:    member.ImageID,
		MemberInfo: member.MemberInfo,
	}

	return info, nil
}

type ListWhereForm struct {
	ListForm
	Order string `json:"order"`
	Where any    `json:"where"`
}

func (f *ListWhereForm) Validate(r *http.Request) error {
	if f == nil || f.Name == "" {
		return errors.New(paramsEmpty)
	}
	return f.ListForm.Validate(r)
}

type blockMetricByNode struct {
	TotalCount   int64 `json:"total_count"`
	PartialCount int64 `json:"partial_count"`
}

func (b *blockChainApi) GetBlocksCountByNode(ctx RequestContext, nodePosition int64, consensusMode int32) (*blockMetricByNode, *Error) {
	if nodePosition < 0 || consensusMode <= 0 || (consensusMode != consts.CandidateNodeMode && consensusMode != consts.HonorNodeMode) {
		return nil, InvalidParamsError(paramsEmpty)
	}
	bk := &sqldb.BlockChain{}
	r := ctx.HTTPRequest()
	logger := getLogger(r)

	found, err := bk.GetMaxBlock()
	if err != nil {
		logger.WithFields(log.Fields{"error": err, "type": consts.DBError}).Error("on getting max block")
		return nil, DefaultError(err.Error())
	}

	if !found {
		return nil, NotFoundError()
	}

	c, err := sqldb.GetBlockCountByNode(nodePosition, consensusMode)
	if err != nil {
		logger := getLogger(r)
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on getting block count by node")
		return nil, InternalError(err.Error())
	}

	bm := blockMetricByNode{TotalCount: bk.ID, PartialCount: c}

	return &bm, nil

}
