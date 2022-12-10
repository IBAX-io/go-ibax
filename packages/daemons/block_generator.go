/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"bytes"
	"context"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/block"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/protocols"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

// BlockGenerator is daemon that generates blocks
func BlockGenerator(ctx context.Context, d *daemon) error {
	if atomic.CompareAndSwapUint32(&d.atomic, 0, 1) {
		defer atomic.StoreUint32(&d.atomic, 0)
	} else {
		return nil
	}
	DBLock()
	defer DBUnlock()
	candidateNodes, err := sqldb.GetCandidateNode(syspar.SysInt(syspar.NumberNodes))
	if err == nil && len(candidateNodes) > 0 {
		syspar.SetRunModel(consts.CandidateNodeMode)
		return BlockGeneratorCandidate(ctx, d)
	}
	syspar.SetRunModel(consts.HonorNodeMode)
	d.sleepTime = time.Second
	if node.IsNodePaused() {
		return nil
	}

	nodePosition, err := syspar.GetThisNodePosition()
	if err != nil {
		// we are not honor node and can't generate new blocks
		d.sleepTime = syspar.GetMaxBlockTimeDuration()
		d.logger.WithFields(log.Fields{"type": consts.JustWaiting, "error": err}).Debug("we are not honor node, sleep for 10 seconds")
		return nil
	}

	// we need fresh myNodePosition after locking
	nodePosition, err = syspar.GetThisNodePosition()
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting node position by key id")
		return err
	}

	btc := protocols.NewBlockTimeCounter()
	st := time.Now()
	if exists, err := btc.BlockForTimeExists(st, int(nodePosition)); exists || err != nil {
		return nil
	}

	timeToGenerate, err := btc.TimeToGenerate(st, int(nodePosition))
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err, "position": nodePosition}).Debug("calculating block time")
		return err
	}

	if !timeToGenerate {
		d.logger.WithFields(log.Fields{"type": consts.JustWaiting}).Debug("not my generation time")
		return nil
	}
	//if !NtpDriftFlag {
	//	d.logger.WithFields(log.Fields{"type": consts.Ntpdate}).Error("ntp time not ntpdate")
	//	return nil
	//}

	//var cf sqldb.Confirmation
	//cfg, err := cf.CheckAllowGenBlock()
	//if err != nil {
	//	d.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Debug("confirmation block not allow")
	//	return err
	//}
	//
	//if !cfg {
	//	d.logger.WithFields(log.Fields{"type": consts.JustWaiting}).Debug("not my confirmation time")
	//	return nil
	//}
	prevBlock := &sqldb.InfoBlock{}
	_, err = prevBlock.Get()
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting previous block")
		return err
	}

	NodePrivateKey, NodePublicKey := utils.GetNodeKeys()
	if len(NodePrivateKey) < 1 {
		d.logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node private key is empty")
		return errors.New(`node private key is empty`)
	}

	dtx := DelayedTx{
		privateKey: NodePrivateKey,
		publicKey:  NodePublicKey,
		logger:     d.logger,
		time:       st.Unix(),
	}

	txs, err := dtx.RunForDelayBlockID(prevBlock.BlockID + 1)
	if err != nil {
		return err
	}

	trs, classifyTxsMap, err := processTransactionsNew(d.logger, txs, st)
	if err != nil {
		return err
	}

	// Block generation will be started only if we have transactions
	if len(trs) == 0 {
		return nil
	}

	header := &types.BlockHeader{
		BlockId:       prevBlock.BlockID + 1,
		Timestamp:     st.Unix(),
		EcosystemId:   0,
		KeyId:         conf.Config.KeyID,
		NetworkId:     conf.Config.LocalConf.NetworkID,
		NodePosition:  nodePosition,
		Version:       consts.BlockVersion,
		ConsensusMode: consts.HonorNodeMode,
	}

	prev := &types.BlockHeader{
		BlockId:       prevBlock.BlockID,
		BlockHash:     prevBlock.Hash,
		RollbacksHash: prevBlock.RollbacksHash,
	}

	err = generateProcessBlockNew(header, prev, trs, classifyTxsMap)
	if err != nil {
		return err
	}
	//go notificator.CheckTokenMovementLimits(nil, conf.Config.TokenMovement, header.BlockId)
	return nil
}

func generateNextBlock(blockHeader, prevBlock *types.BlockHeader, trs [][]byte) ([]byte, error) {
	return block.MarshallBlock(
		types.WithCurHeader(blockHeader),
		types.WithPrevHeader(prevBlock),
		types.WithTxFullData(trs))
}

func processTransactionsNew(logger *log.Entry, txs []*sqldb.Transaction, st time.Time) ([][]byte, map[int][]*transaction.Transaction, error) {
	classifyTxsMap := make(map[int][]*transaction.Transaction)
	var done = make(<-chan time.Time, 1)
	if syspar.IsHonorNodeMode() {
		btc := protocols.NewBlockTimeCounter()
		_, endTime, err := btc.RangeByTime(st)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.TimeCalcError, "error": err}).Error("on getting end time of generation")
			return nil, nil, err
		}
		done = time.After(endTime.Sub(st))
	}
	trs, err := sqldb.GetAllUnusedTransactions(nil, syspar.GetMaxTxCount()-len(txs))
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting all unused transactions")
		return nil, nil, err
	}

	limits := transaction.NewLimits(transaction.GetLetPreprocess())

	type badTxStruct struct {
		hash  []byte
		msg   string
		keyID int64
	}

	processBadTx := func(dbTx *sqldb.DbTransaction) chan badTxStruct {
		ch := make(chan badTxStruct)

		go func() {
			for badTxItem := range ch {
				transaction.BadTxForBan(badTxItem.keyID)
				_ = transaction.MarkTransactionBad(badTxItem.hash, badTxItem.msg)
			}
		}()

		return ch
	}

	txBadChan := processBadTx(nil)

	defer func() {
		close(txBadChan)
	}()

	// Checks preprocessing count limits
	txList := make([][]byte, 0, len(trs))
	txs = append(txs, trs...)

	allDelayedContract, err := sqldb.GetAllDelayedContract()
	if err != nil {
		return nil, nil, err
	}
	var contractNames []string
	for _, contract := range allDelayedContract {
		contractNames = append(contractNames, contract.Contract)
	}

	for i, txItem := range txs {
		if syspar.IsHonorNodeMode() {
			select {
			case <-done:
				return txList, classifyTxsMap, nil
			default:
			}
		}
		bufTransaction := bytes.NewBuffer(txItem.Data)
		tr, err := transaction.UnmarshallTransaction(bufTransaction, true)
		if err != nil {
			if tr != nil {
				txBadChan <- badTxStruct{hash: tr.Hash(), msg: err.Error(), keyID: tr.KeyID()}
			}
			continue
		}

		if err := tr.Check(st.Unix()); err != nil {
			txBadChan <- badTxStruct{hash: tr.Hash(), msg: err.Error(), keyID: tr.KeyID()}
			continue
		}
		if txItem.GetTransactionRateStopNetwork() {
			classifyTxsMap[types.StopNetworkTxType] = append(classifyTxsMap[types.StopNetworkTxType], tr)
			txList = append(txList[:0], txs[i].Data)
			break
		}
		if tr.IsSmartContract() {
			err = limits.CheckLimit(tr.Inner)
			if errors.Cause(err) == transaction.ErrLimitStop && i > 0 {
				break
			} else if err != nil {
				if err != transaction.ErrLimitSkip {
					txBadChan <- badTxStruct{hash: tr.Hash(), msg: err.Error(), keyID: tr.KeyID()}
				}
				continue
			}
			if tr.Type() == types.TransferSelfTxType {
				classifyTxsMap[types.TransferSelfTxType] = append(classifyTxsMap[types.TransferSelfTxType], tr)
				txList = append(txList, txs[i].Data)
				continue
			}
			if tr.Type() == types.UtxoTxType {
				classifyTxsMap[types.UtxoTxType] = append(classifyTxsMap[types.UtxoTxType], tr)
				txList = append(txList, txs[i].Data)
				continue
			}

			if utils.StringInSlice(contractNames, tr.SmartContract().TxContract.Name) {
				classifyTxsMap[types.DelayTxType] = append(classifyTxsMap[types.DelayTxType], tr)
				txList = append(txList, txs[i].Data)
				continue
			}
			classifyTxsMap[types.SmartContractTxType] = append(classifyTxsMap[types.SmartContractTxType], tr)
		}
		txList = append(txList, txs[i].Data)
	}
	return txList, classifyTxsMap, nil
}
