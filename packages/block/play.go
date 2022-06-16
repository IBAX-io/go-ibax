/*----------------------------------------------------------------
- Copyright (c) IBAX. All rights reserved.
- See LICENSE in the project root for license information.
---------------------------------------------------------------*/

package block

import (
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
	"github.com/IBAX-io/go-ibax/packages/utxo"
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
		if b.GenBlock && len(b.Transactions) == 0 {
			if err == transaction.ErrLimitStop {
				err = script.ErrVMTimeLimit
			}
			if inputTx[0].IsSmartContract() {
				transaction.BadTxForBan(inputTx[0].KeyID())
			}
			if err := transaction.MarkTransactionBad(dbTx, inputTx[0].Hash(), err.Error()); err != nil {
				return err
			}
		}
		return err
	}

	if b.GenBlock {
		if len(b.Transactions) == 0 {
			dbTx.Commit()
			return ErrEmptyBlock
		}
		b.Header.RollbacksHash, err = b.GetRollbacksHash(dbTx)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error("getting rollbacks hash")
			return err
		}
		if err = b.repeatMarshallBlock(); err != nil {
			dbTx.Rollback()
			return err
		}
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
	//logger := b.GetLogger()
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
		return nil
	}
	var keyIds []int64
	for indexTx := 0; indexTx < len(b.Transactions); indexTx++ {
		t := b.Transactions[indexTx]
		// fmt.Println("KeyID", t.KeyID())
		if t.IsSmartContract() && t.SmartContract().TxSmart.Utxo != nil {
			// fmt.Println("ToId", t.SmartContract().TxSmart.Utxo.ToID)
		}
		keyIds = append(keyIds, t.KeyID())
	}
	// GroupTxs KeyID ToID
	outputs, err := sqldb.GetTxOutputs(dbTx, keyIds)
	if err != nil {
		return err
	}
	//fmt.Println("outputs", outputs)
	utxo.PutAllOutputsMap(outputs)
	// b.TxOutputs
	// b.TxInputs

	walletAddress := make(map[int64]int64)
	groupTxs(b.Transactions, walletAddress)
	// exec GroupTxs

	for {
		execGroupMap := make(map[string][]*transaction.Transaction)
		for s, txs := range txsGroupMap {
			if (!txs[0].IsSmartContract() || txs[0].SmartContract().TxSmart.Utxo == nil) && len(execGroupMap) == 0 {
				execGroupMap[s] = txs
				delete(txsGroupMap, s)
				break
			} else if (!txs[0].IsSmartContract() || txs[0].SmartContract().TxSmart.Utxo == nil) && len(execGroupMap) > 0 {
				break
			} else {
				execGroupMap[s] = txs
				delete(txsGroupMap, s)
				continue
			}
		}
		var wg sync.WaitGroup
		for key, transactions := range execGroupMap {
			wg.Add(1)
			//dbTx2, err2 := sqldb.StartTransaction2(dbTx.Connection())
			//if err2 != nil {
			//	logger.WithFields(log.Fields{"type": consts.DBError, "error": err2}).Error("starting db transaction2")
			//	return err2
			//}
			go func(k string, _transactions []*transaction.Transaction, _dbTx2 *sqldb.DbTransaction) {
				defer wg.Done()
				for curTx := 0; curTx < len(_transactions); curTx++ {
					tx := _transactions[curTx]

					after, t, err1 := b.executeTx(k, tx, _dbTx2, rand, limits, curTx)

					if err1 != nil {
						_dbTx2.Rollback()

						break
					}

					afters.Txs = append(afters.Txs, after)
					afters.Rts = append(afters.Rts, t.RollBackTx...)
					afters.TxExecutionSql = append(afters.TxExecutionSql, t.DbTransaction.ExecutionSql...)
					processedTx = append(processedTx, t.FullData)
				}
			}(key, transactions, dbTx)
		}
		wg.Wait()

		if len(txsGroupMap) == 0 {
			log.Printf("执行分组结束")
			break
		}
	}
	return nil
}
func (b *Block) executeTx(k string, t *transaction.Transaction, _dbTx2 *sqldb.DbTransaction, rand *random.Rand, limits *transaction.Limits, curTx int) (*types.AfterTx, *transaction.Transaction, error) {
	logger := b.GetLogger()
	log.Printf("执行分组 = %v,执行交易=%+v\n", k, t)

	var txInputs []sqldb.SpentInfo
	if t.IsSmartContract() {

		txInputs = utxo.GetUnusedOutputsMap(t.KeyID())
	}

	err := _dbTx2.Savepoint(consts.SetSavePointMarkBlock(curTx))
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": t.Hash()}).Error("using savepoint")
		return nil, nil, err
	}
	err = t.WithOption(notificator.NewQueue(), b.GenBlock, b.Header, b.PrevHeader, _dbTx2, rand.BytesSeed(t.Hash()), limits, curTx, txInputs)
	if err != nil {
		return nil, nil, err
	}
	err = t.Play()
	if err != nil {
		if err == transaction.ErrNetworkStopping {
			// Set the node in a pause state
			node.PauseNodeActivity(node.PauseTypeStopingNetwork)
			return nil, nil, err
		}
		errRoll := t.DbTransaction.RollbackSavepoint(consts.SetSavePointMarkBlock(curTx))
		if errRoll != nil {
			t.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": t.Hash()}).Error("rolling back to previous savepoint")
			return nil, nil, errRoll
		}
		if b.GenBlock {
			if err == transaction.ErrLimitStop {
				if curTx == 0 {
					return nil, nil, err
				}
				//break
			}
			if strings.Contains(err.Error(), script.ErrVMTimeLimit.Error()) {
				err = script.ErrVMTimeLimit
			}
		}
		if t.IsSmartContract() {
			transaction.BadTxForBan(t.KeyID())
		}
		_ = transaction.MarkTransactionBad(t.DbTransaction, t.Hash(), err.Error())
		if t.SysUpdate {
			if err := syspar.SysUpdate(t.DbTransaction); err != nil {
				log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating syspar")
				return nil, nil, err
			}
			t.SysUpdate = false
		}
		if b.GenBlock {
			//continue
		}
		return nil, nil, err
	}

	if t.IsSmartContract() && len(t.TxOutputs) > 0 {
		utxo.UpdateTxInputs(t.Hash(), txInputs)
		utxo.InsertTxOutputs(t.Hash(), t.TxOutputs)
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
		contract = t.SmartContract().TxContract.Name
		code = t.TxResult.Code
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

	return after, t, nil
}

var (
	txsGroupMap         = make(map[string][]*transaction.Transaction)
	groupTxsList        = make([]*transaction.Transaction, 0)
	groupSerial  uint16 = 1
)

func groupTxs(txs []*transaction.Transaction, walletAddress map[int64]int64) map[string][]*transaction.Transaction {
	if len(txs) == 0 {
		return txsGroupMap
	}
	crrentGroupTxsSize := len(groupTxsList)
	size := len(txs)
	for i := 0; i < size; i++ {
		if len(walletAddress) == 0 {

			if !txs[i].IsSmartContract() || txs[i].SmartContract().TxSmart.Utxo == nil {
				walletAddress[txs[i].KeyID()] = txs[i].KeyID()
				//walletAddress[txs[i].to] = txs[i].to

				groupTxsList = append(groupTxsList, txs[i])
				txs = txs[1:]
				size = len(txs)

				break
			}
			//walletAddress[txs[i].KeyID()] = txs[i].KeyID()
			// TODO  Utxo.ToID maybe nil
			if txs[i].SmartContract().TxSmart.Utxo != nil {
				walletAddress[txs[i].SmartContract().TxSmart.Utxo.ToID] = txs[i].SmartContract().TxSmart.Utxo.ToID
			}

			groupTxsList = append(groupTxsList, txs[i])
			txs = txs[1:]
			size = len(txs)
			i--
			continue
		}
		if txs[i].IsSmartContract() && (walletAddress[txs[i].KeyID()] != 0 || walletAddress[txs[i].SmartContract().TxSmart.Utxo.ToID] != 0) {
			walletAddress[txs[i].KeyID()] = txs[i].KeyID()
			walletAddress[txs[i].SmartContract().TxSmart.Utxo.ToID] = txs[i].SmartContract().TxSmart.Utxo.ToID

			groupTxsList = append(groupTxsList, txs[i])
			txs = append(txs[:i], txs[i+1:]...)
			size = len(txs)
			i--
		} else {
			break
		}
	}

	if len(groupTxsList) == 1 && (!groupTxsList[0].IsSmartContract() || groupTxsList[0].SmartContract().TxSmart.Utxo == nil) {
		tempGroupTxsList := groupTxsList
		txsGroupMap[strconv.Itoa(int(groupSerial))] = tempGroupTxsList

		groupSerial++
		groupTxsList = make([]*transaction.Transaction, 0)
		walletAddress = make(map[int64]int64)

		return groupTxs(txs, walletAddress)
	}

	if crrentGroupTxsSize < len(groupTxsList) {
		if len(txs) == 0 {
			txsGroupMap[strconv.Itoa(int(groupSerial))] = groupTxsList
			return txsGroupMap
		}
		return groupTxs(txs, walletAddress)
	}

	if len(groupTxsList) > 0 {
		tempGroupTxsList := groupTxsList
		txsGroupMap[strconv.Itoa(int(groupSerial))] = tempGroupTxsList
		groupSerial++
		groupTxsList = make([]*transaction.Transaction, 0)
		walletAddress = make(map[int64]int64)
	}

	return groupTxs(txs, walletAddress)
}
