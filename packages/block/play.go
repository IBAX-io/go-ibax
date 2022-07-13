/*----------------------------------------------------------------
- Copyright (c) IBAX. All rights reserved.
- See LICENSE in the project root for license information.
---------------------------------------------------------------*/

package block

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/IBAX-io/go-ibax/packages/common/random"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/notificator"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// PlaySafe is inserting block safely
func (b *Block) PlaySafe() error {
	logger := b.GetLogger()
	dbTx, err := sqldb.StartTransaction()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("starting db transaction")
		return err
	}

	inputTx := b.Transactions[:]
	err = b.ProcessTxs(dbTx)
	if err != nil {
		dbTx.Rollback()
		if b.GenBlock && len(b.TxFullData) == 0 {
			if inputTx[0].IsSmartContract() {
				transaction.BadTxForBan(inputTx[0].KeyID())
			}
			if err := transaction.MarkTransactionBad(inputTx[0].Hash(), err.Error()); err != nil {
				return err
			}
		}
		return err
	}

	if b.GenBlock && len(b.TxFullData) == 0 {
		dbTx.Commit()
		return ErrEmptyBlock
	}

	if err := b.InsertIntoBlockchain(dbTx); err != nil {
		dbTx.Rollback()
		return err
	}
	err = dbTx.Commit()
	if err != nil {
		return err
	}
	for _, q := range b.Notifications {
		q.Send()
	}
	return nil
}

func (b *Block) ProcessTxs(dbTx *sqldb.DbTransaction) (err error) {
	afters := &types.AfterTxs{
		Rts: make([]*types.RollbackTx, 0),
		Txs: make([]*types.AfterTx, 0),
	}
	logger := b.GetLogger()
	limits := transaction.NewLimits(b.limitMode())
	rand := random.NewRand(b.Header.Timestamp)
	processedTx := make([][]byte, 0, len(b.Transactions))
	defer func() {
		if b.IsGenesis() || b.GenBlock {
			b.AfterTxs = afters
		}
		if b.GenBlock {
			b.TxFullData = processedTx
		}
		if errA := b.AfterPlayTxs(dbTx); errA != nil {
			if err == nil {
				err = errA
			} else if err != nil {
				err = fmt.Errorf("%v; %w", err, errA)
			}
			return
		}
	}()
	if !b.GenBlock && !b.IsGenesis() && conf.Config.BlockSyncMethod.Method == types.BlockSyncMethod_SQLDML.String() {
		if b.SysUpdate {
			if err := syspar.SysUpdate(dbTx); err != nil {
				return fmt.Errorf("updating syspar: %w", err)
			}
		}
		return nil
	}

	var keyIds []int64
	for indexTx := 0; indexTx < len(b.Transactions); indexTx++ {
		t := b.Transactions[indexTx]
		keyIds = append(keyIds, t.KeyID())
	}
	outputs, err := sqldb.GetTxOutputs(dbTx, keyIds)
	if err != nil {
		return err
	}
	b.OutputsMap = make(map[int64][]sqldb.SpentInfo)
	sqldb.PutAllOutputsMap(outputs, b.OutputsMap)

	txsMap := b.ClassifyTxsMap
	//trs := []*sqldb.Transaction{}
	transactions := make([]*transaction.Transaction, 0)
	var wg sync.WaitGroup
	for {
		// StopNetworkTxType
		if len(txsMap[consts.StopNetworkTxType]) > 0 {
			for _, txBytes := range txsMap[consts.StopNetworkTxType] {
				t, err := transaction.UnmarshallTransaction(bytes.NewBuffer(txBytes))
				if err != nil {
					if t != nil && t.Hash() != nil {
						transaction.MarkTransactionBad(t.Hash(), err.Error())
					}
					return fmt.Errorf("parse transaction error(%s)", err)
				}
				transactions = append(transactions, t)
			}
			err := b.serialExecuteTxs(dbTx, logger, rand, limits, afters, &processedTx, transactions)
			delete(txsMap, consts.StopNetworkTxType)
			transactions = make([]*transaction.Transaction, 0)
			if err != nil {
				return err
			}
		}

		// FirstBlockTxType
		if b.IsGenesis() {
			for _, tx := range b.Transactions {
				t, err := transaction.UnmarshallTransaction(bytes.NewBuffer(tx.FullData))
				if err != nil {
					if t != nil && t.Hash() != nil {
						transaction.MarkTransactionBad(t.Hash(), err.Error())
					}
					return fmt.Errorf("parse transaction error(%s)", err)
				}
				transactions = append(transactions, t)
			}
			err := b.serialExecuteTxs(dbTx, logger, rand, limits, afters, &processedTx, transactions)
			transactions = make([]*transaction.Transaction, 0)
			if err != nil {
				return err
			}
		}

		// DelayTxType
		if len(txsMap[consts.DelayTxType]) > 0 {
			for _, txBytes := range txsMap[consts.DelayTxType] {
				t, err := transaction.UnmarshallTransaction(bytes.NewBuffer(txBytes))
				if err != nil {
					if t != nil && t.Hash() != nil {
						transaction.MarkTransactionBad(t.Hash(), err.Error())
					}
					return fmt.Errorf("parse transaction error(%s)", err)
				}
				transactions = append(transactions, t)
			}
			err := b.serialExecuteTxs(dbTx, logger, rand, limits, afters, &processedTx, transactions)
			delete(txsMap, consts.DelayTxType)
			transactions = make([]*transaction.Transaction, 0)
			if err != nil {
				return err
			}
		}

		// TransferSelf
		if len(txsMap[consts.TransferSelf]) > 0 {
			for curTx, txBytes := range txsMap[consts.TransferSelf] {
				wg.Add(1)
				t, err := transaction.UnmarshallTransaction(bytes.NewBuffer(txBytes))
				if err != nil {
					if t != nil && t.Hash() != nil {
						transaction.MarkTransactionBad(t.Hash(), err.Error())
					}
					return fmt.Errorf("parse transaction error(%s)", err)
				}
				transactions = append(transactions, t)
				go func() {
					defer wg.Done()
					err := b.executeSingleTx(dbTx, logger, rand, limits, afters, &processedTx, t, curTx)
					if err != nil {
						return
					}
				}()
			}
			wg.Wait()
			delete(txsMap, consts.TransferSelf)
			transactions = make([]*transaction.Transaction, 0)
		}

		// SmartContractTxType
		if len(txsMap[consts.SmartContractTxType]) > 0 {
			for _, txBytes := range txsMap[consts.SmartContractTxType] {
				t, err := transaction.UnmarshallTransaction(bytes.NewBuffer(txBytes))
				if err != nil {
					if t != nil && t.Hash() != nil {
						transaction.MarkTransactionBad(t.Hash(), err.Error())
					}
					return fmt.Errorf("parse transaction error(%s)", err)
				}
				transactions = append(transactions, t)
			}
			err := b.serialExecuteTxs(dbTx, logger, rand, limits, afters, &processedTx, transactions)
			delete(txsMap, consts.SmartContractTxType)
			transactions = make([]*transaction.Transaction, 0)
			if err != nil {
				return err
			}
		}

		//Utxo
		if len(txsMap[consts.Utxo]) > 0 {
			for _, txBytes := range txsMap[consts.Utxo] {
				t, err := transaction.UnmarshallTransaction(bytes.NewBuffer(txBytes))
				if err != nil {
					if t != nil && t.Hash() != nil {
						transaction.MarkTransactionBad(t.Hash(), err.Error())
					}
					return fmt.Errorf("parse transaction error(%s)", err)
				}
				transactions = append(transactions, t)
			}

			// utxo group
			walletAddress := make(map[int64]int64)
			groupUtxoTxs(transactions, walletAddress)

			for {
				for g, transactions := range utxoTxsGroupMap {
					wg.Add(1)
					go func() {
						defer wg.Done()
						err := b.serialExecuteTxs(dbTx, logger, rand, limits, afters, &processedTx, transactions)
						delete(utxoTxsGroupMap, g)
						if err != nil {
							return
						}
					}()
				}
				if len(utxoTxsGroupMap) == 0 {
					log.Printf("excute UTXO txs finished")
					break
				}
			}
			wg.Wait()

			delete(txsMap, consts.Utxo)
			transactions = make([]*transaction.Transaction, 0)
		}

		if len(txsMap) == 0 {
			log.Printf("all txs excute finished ")
			break
		}
	}
	return nil
}

func (b *Block) serialExecuteTxs(dbTx *sqldb.DbTransaction, logger *log.Entry, rand *random.Rand, limits *transaction.Limits, afters *types.AfterTxs, processedTx *[][]byte, txs []*transaction.Transaction) error {
	for curTx := 0; curTx < len(txs); curTx++ {
		t := b.Transactions[curTx]
		err := dbTx.Savepoint(consts.SetSavePointMarkBlock(curTx))
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": t.Hash()}).Error("using savepoint")
			return err
		}
		err = t.WithOption(notificator.NewQueue(), b.GenBlock, b.Header, b.PrevHeader, dbTx, rand.BytesSeed(t.Hash()), limits, curTx, b.OutputsMap)
		if err != nil {
			return err
		}
		err = t.Play()
		if err != nil {
			if err == transaction.ErrNetworkStopping {
				// Set the node in a pause state
				node.PauseNodeActivity(node.PauseTypeStopingNetwork)
				return err
			}
			errRoll := t.DbTransaction.RollbackSavepoint(consts.SetSavePointMarkBlock(t.InToCxt.SqlDbSavePoint))
			if errRoll != nil {
				return fmt.Errorf("%v; %w", err, errRoll)
			}
			if b.GenBlock {
				if errors.Cause(err) == transaction.ErrLimitStop {
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
				transaction.BadTxForBan(t.KeyID())
			}
			_ = transaction.MarkTransactionBad(t.Hash(), err.Error())
			if t.SysUpdate {
				if err := syspar.SysUpdate(t.DbTransaction); err != nil {
					return fmt.Errorf("updating syspar: %w", err)
				}
				t.SysUpdate = false
			}
			if b.GenBlock {
				continue
			}

		}

		if t.SysUpdate {
			b.SysUpdate = true
			t.SysUpdate = false
		}

		if t.Notifications.Size() > 0 {
			b.Notifications = append(b.Notifications, t.Notifications)
		}

		var (
			after    = &types.AfterTx{}
			eco      int64
			contract string
			code     pbgo.TxInvokeStatusCode
		)
		if t.IsSmartContract() {
			eco = t.SmartContract().TxSmart.EcosystemID
			code = t.TxResult.Code
			if t.SmartContract().TxContract != nil {
				contract = t.SmartContract().TxContract.Name
			}
		}
		after.UsedTx = t.Hash()
		after.Lts = &types.LogTransaction{
			Block:        t.BlockHeader.BlockId,
			Hash:         t.Hash(),
			TxData:       t.FullData,
			Timestamp:    t.Timestamp(),
			Address:      t.KeyID(),
			EcosystemId:  eco,
			ContractName: contract,
			InvokeStatus: code,
		}
		after.UpdTxStatus = t.TxResult
		afters.Txs = append(afters.Txs, after)
		afters.Rts = append(afters.Rts, t.RollBackTx...)
		afters.TxBinLogSql = append(afters.TxBinLogSql, t.DbTransaction.BinLogSql...)
		*processedTx = append(*processedTx, t.FullData)

		sqldb.UpdateTxInputs(t.Hash(), t.TxInputs, b.OutputsMap)
		sqldb.InsertTxOutputs(t.Hash(), t.TxOutputs, b.OutputsMap)
	}
	return nil
}

func (b *Block) executeSingleTx(dbTx *sqldb.DbTransaction, logger *log.Entry, rand *random.Rand, limits *transaction.Limits, afters *types.AfterTxs, processedTx *[][]byte, tx *transaction.Transaction, curTx int) error {
	t := tx
	err := dbTx.Savepoint(consts.SetSavePointMarkBlock(curTx))
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": t.Hash()}).Error("using savepoint")
		return err
	}
	err = t.WithOption(notificator.NewQueue(), b.GenBlock, b.Header, b.PrevHeader, dbTx, rand.BytesSeed(t.Hash()), limits, curTx, b.OutputsMap)
	if err != nil {
		return err
	}
	err = t.Play()
	if err != nil {
		if err == transaction.ErrNetworkStopping {

			node.PauseNodeActivity(node.PauseTypeStopingNetwork)
			return err
		}
		errRoll := t.DbTransaction.RollbackSavepoint(consts.SetSavePointMarkBlock(t.InToCxt.SqlDbSavePoint))
		if errRoll != nil {
			return fmt.Errorf("%v; %w", err, errRoll)
		}
		if b.GenBlock {
			if errors.Cause(err) == transaction.ErrLimitStop {
				if curTx == 0 {
					return err
				}
				//break
				return nil
			}
			if strings.Contains(err.Error(), script.ErrVMTimeLimit.Error()) {
				err = script.ErrVMTimeLimit
			}
		}
		if t.IsSmartContract() {
			transaction.BadTxForBan(t.KeyID())
		}
		_ = transaction.MarkTransactionBad(t.Hash(), err.Error())
		if t.SysUpdate {
			if err := syspar.SysUpdate(t.DbTransaction); err != nil {
				return fmt.Errorf("updating syspar: %w", err)
			}
			t.SysUpdate = false
		}
		if b.GenBlock {
			//continue
			return nil
		}

	}

	if t.SysUpdate {
		b.SysUpdate = true
		t.SysUpdate = false
	}

	if t.Notifications.Size() > 0 {
		b.Notifications = append(b.Notifications, t.Notifications)
	}

	var (
		after    = &types.AfterTx{}
		eco      int64
		contract string
		code     pbgo.TxInvokeStatusCode
	)
	if t.IsSmartContract() {
		eco = t.SmartContract().TxSmart.EcosystemID
		code = t.TxResult.Code
		if t.SmartContract().TxContract != nil {
			contract = t.SmartContract().TxContract.Name
		}
	}
	after.UsedTx = t.Hash()
	after.Lts = &types.LogTransaction{
		Block:        t.BlockHeader.BlockId,
		Hash:         t.Hash(),
		TxData:       t.FullData,
		Timestamp:    t.Timestamp(),
		Address:      t.KeyID(),
		EcosystemId:  eco,
		ContractName: contract,
		InvokeStatus: code,
	}
	after.UpdTxStatus = t.TxResult
	afters.Txs = append(afters.Txs, after)
	afters.Rts = append(afters.Rts, t.RollBackTx...)
	afters.TxBinLogSql = append(afters.TxBinLogSql, t.DbTransaction.BinLogSql...)
	*processedTx = append(*processedTx, t.FullData)

	sqldb.UpdateTxInputs(t.Hash(), t.TxInputs, b.OutputsMap)
	sqldb.InsertTxOutputs(t.Hash(), t.TxOutputs, b.OutputsMap)

	return nil
}

var (
	utxoTxsGroupMap         = make(map[string][]*transaction.Transaction)
	utxoGroupTxsList        = make([]*transaction.Transaction, 0)
	utxoGroupSerial  uint16 = 1
)

func groupUtxoTxs(txs []*transaction.Transaction, walletAddress map[int64]int64) map[string][]*transaction.Transaction {
	if len(txs) == 0 {
		return utxoTxsGroupMap
	}
	crrentGroupTxsSize := len(utxoGroupTxsList)
	size := len(txs)
	for i := 0; i < size; i++ {
		if len(walletAddress) == 0 {
			walletAddress[txs[i].KeyID()] = txs[i].KeyID()
			walletAddress[txs[i].SmartContract().TxSmart.UTXO.ToID] = txs[i].SmartContract().TxSmart.UTXO.ToID

			utxoGroupTxsList = append(utxoGroupTxsList, txs[i])
			txs = txs[1:]
			size = len(txs)
			i--
			continue
		}
		if walletAddress[txs[i].KeyID()] != 0 || walletAddress[txs[i].SmartContract().TxSmart.UTXO.ToID] != 0 {
			walletAddress[txs[i].KeyID()] = txs[i].KeyID()
			walletAddress[txs[i].SmartContract().TxSmart.UTXO.ToID] = txs[i].SmartContract().TxSmart.UTXO.ToID

			utxoGroupTxsList = append(utxoGroupTxsList, txs[i])
			txs = append(txs[:i], txs[i+1:]...)
			size = len(txs)
			i--
		}
	}

	if crrentGroupTxsSize < len(utxoGroupTxsList) {
		if len(txs) == 0 {
			utxoTxsGroupMap[strconv.Itoa(int(utxoGroupSerial))] = utxoGroupTxsList
			return utxoTxsGroupMap
		}
		return groupUtxoTxs(txs, walletAddress)
	}

	if len(utxoGroupTxsList) > 0 {
		tempUtxoGroupTxsList := utxoGroupTxsList
		utxoTxsGroupMap[strconv.Itoa(int(utxoGroupSerial))] = tempUtxoGroupTxsList
		utxoGroupSerial++
		utxoGroupTxsList = make([]*transaction.Transaction, 0)
		walletAddress = make(map[int64]int64)
	}

	return groupUtxoTxs(txs, walletAddress)
}
