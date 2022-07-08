/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"sync/atomic"
	"time"

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

	trs, err := transaction.ProcessTransactions(d.logger, txs, st)
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
		NodePosition:  nodePosition,
		Version:       consts.BlockVersion,
		ConsensusMode: consts.HonorNodeMode,
	}

	prev := &types.BlockHeader{
		BlockId:       prevBlock.BlockID,
		BlockHash:     prevBlock.Hash,
		RollbacksHash: prevBlock.RollbacksHash,
	}

	err = generateProcessBlock(header, prev, trs)
	if err != nil {
		return err
	}
	//go notificator.CheckTokenMovementLimits(nil, conf.Config.TokenMovement, header.BlockId)
	return nil
}
