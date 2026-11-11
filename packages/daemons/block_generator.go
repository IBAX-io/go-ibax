/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/block"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/protocols"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

// honorPoll is how often a node of a chain of honor nodes looks for the transactions of a block
const honorPoll = 100 * time.Millisecond

// lastGenerated is the block the node generated last, the time it ended and whether the pool still
// held transactions then. The stages lead and cycle of the next block are measured from it when the
// pool held transactions, so the interval of the blocks a node makes in a row under a load is
// measured on the node and not by a client that polls the chain, and a wait for transactions is not
// counted
var lastGenerated struct {
	id      int64
	end     time.Time
	backlog bool
}

// BlockGenerator is daemon that generates blocks
func BlockGenerator(ctx context.Context, d *daemon) error {
	if atomic.CompareAndSwapUint32(&d.atomic, 0, 1) {
		defer atomic.StoreUint32(&d.atomic, 0)
	} else {
		return nil
	}
	DBLock()
	defer DBUnlock()
	if syspar.IsBftConsensus() {
		// the consensus driver makes the blocks of the chain
		startBftConsensus(ctx, d.logger)
		d.sleepTime = time.Second
		return nil
	}
	candidateNodes, err := syspar.GetCandidateNodes()
	if err == nil && len(candidateNodes) > 0 {
		syspar.SetRunModel(consts.CandidateNodeMode)
		return BlockGeneratorCandidate(ctx, d)
	}
	syspar.SetRunModel(consts.HonorNodeMode)
	// a node makes the block of a slot as soon as it has transactions for it (plan 5.1)
	d.sleepTime = honorPoll
	if node.IsNodePaused() {
		return nil
	}

	nodePosition, err := syspar.GetThisNodePosition()
	if err != nil {
		// we are not honor node and can't generate new blocks
		d.sleepTime = syspar.GetHonorSlot()
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

	_, parsed, err := processTransactionsNew(d.logger, txs, st)
	if err != nil {
		return err
	}
	if len(parsed) > 0 {
		metrics.BlockStage.WithLabelValues(metrics.KindGenerate, metrics.StageCollect).Observe(metrics.Since(st))
	}

	// Block generation will be started only if we have transactions
	if len(parsed) == 0 {
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

	err = generateProcessBlockNew(header, parsed)
	if err != nil {
		return err
	}
	end := time.Now()
	if lastGenerated.backlog && lastGenerated.id+1 == header.BlockId {
		metrics.BlockStage.WithLabelValues(metrics.KindGenerate, metrics.StageLead).Observe(st.Sub(lastGenerated.end).Seconds())
		metrics.BlockStage.WithLabelValues(metrics.KindGenerate, metrics.StageCycle).Observe(end.Sub(lastGenerated.end).Seconds())
	}
	lastGenerated.id, lastGenerated.end, lastGenerated.backlog = header.BlockId, end, transaction.TxPool.Len() > 0
	// the transactions left in the queue may make the block of the next slot without a wait
	d.sleepTime = 0
	//go notificator.CheckTokenMovementLimits(nil, conf.Config.TokenMovement, header.BlockId)
	return nil
}

func generateNextBlock(blockHeader, prevBlock *types.BlockHeader, trs [][]byte) ([]byte, error) {
	return block.MarshallBlock(
		types.WithCurHeader(blockHeader),
		types.WithPrevHeader(prevBlock),
		types.WithTxFullData(trs))
}

// processTransactionsNew takes the transactions of a new block: the delayed ones of txs first, then
// those of the pool. It returns their data and the parsed transactions, in the order the block
// executes and stores them
func processTransactionsNew(logger *log.Entry, txs []*transaction.Pending, st time.Time) ([][]byte, []*transaction.Transaction, error) {
	var done = make(<-chan time.Time, 1)
	limited := syspar.IsHonorNodeMode() || syspar.IsBftConsensus()
	if limited {
		// a node takes transactions for half of max_block_generation_time, and executes them in
		// about as long, so it makes the block within max_block_generation_time: the time a
		// proposer of a chain of the BFT consensus has (syspar.GetBftTimeliness), and the time a
		// block of a chain of honor nodes takes, which may span slots, as a slot holds at most
		// one block and the next block takes the next free slot
		done = time.After(time.Duration(syspar.GetMaxBlockGenerationTime()) * time.Millisecond / 2)
	}
	trs := transaction.TxPool.Take(syspar.GetMaxTxCount() - len(txs))

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
				transaction.MarkTransactionBad(badTxItem.hash, badTxItem.msg)
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
	taken := make([]*transaction.Transaction, 0, len(txs))

	parsed := make([]*transaction.Transaction, len(txs))
	for i, txItem := range txs {
		parsed[i] = txItem.Tx
	}
	checked := transaction.CheckBatch(parsed, st.Unix())
	for i, txItem := range txs {
		if limited {
			select {
			case <-done:
				return txList, taken, nil
			default:
			}
		}
		tr := txItem.Tx
		if err := checked[i]; errors.Is(err, transaction.ErrDuplicatedTx) {
			// a block of the node took it after the pool handed it out: it is done, not bad
			transaction.TxPool.Remove([][]byte{tr.Hash()})
			continue
		} else if err != nil {
			txBadChan <- badTxStruct{hash: tr.Hash(), msg: err.Error(), keyID: tr.KeyID()}
			continue
		}
		if txItem.Rate == transaction.RateStopNetwork {
			// a StopNetwork transaction is alone in its block
			return [][]byte{txItem.Data}, []*transaction.Transaction{tr}, nil
		}
		if tr.IsSmartContract() {
			err := limits.CheckLimit(tr.Inner)
			if errors.Cause(err) == transaction.ErrLimitStop && i > 0 {
				break
			} else if err != nil {
				if err != transaction.ErrLimitSkip {
					txBadChan <- badTxStruct{hash: tr.Hash(), msg: err.Error(), keyID: tr.KeyID()}
				}
				continue
			}
		}
		txList = append(txList, txItem.Data)
		taken = append(taken, tr)
	}
	return txList, taken, nil
}
