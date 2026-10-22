/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"sync/atomic"

	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"
	"github.com/IBAX-io/go-ibax/packages/service/node"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"

	log "github.com/sirupsen/logrus"
)

// Disseminator is send to all nodes from nodes_connections the following data
// if we are honor node: sends blocks and transactions hashes
// else send the full transactions
func Disseminator(ctx context.Context, d *daemon) error {
	if atomic.CompareAndSwapUint32(&d.atomic, 0, 1) {
		defer atomic.StoreUint32(&d.atomic, 0)
	} else {
		return nil
	}

	var (
		isHonorNode = true
		selectMode  = SelectModel{}
	)
	myNodePosition, err := selectMode.GetThisNodePosition()

	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Debug("finding node")
		isHonorNode = false
	}

	if isHonorNode {
		// send blocks and transactions hashes
		d.logger.Debug("we are honor_node, sending hashes")
		return sendBlockWithTxHashes(ctx, myNodePosition, d.logger)
	}

	// we are not honor node for this StateID and WalletID, so just send transactions
	d.logger.Debug("we are honor_node, sending transactions")
	return sendTransactions(ctx, d.logger)
}

func sendTransactions(ctx context.Context, logger *log.Entry) error {
	trs := unsentTransactions()
	if len(trs) == 0 {
		logger.Info("transactions not found")
		return nil
	}

	hosts := make([]string, 0)
	if syspar.IsHonorNodeMode() {
		hosts = syspar.GetDefaultRemoteHosts()
	} else {
		candidateNodes, err := GetCandidateNodes()
		if err != nil {
			return err
		}
		for _, node := range candidateNodes {
			hosts = append(hosts, node.TcpAddress)
		}
	}

	if err := tcpclient.SendTransacitionsToAll(ctx, hosts, trs); err != nil {
		log.WithFields(log.Fields{"type": consts.NetworkError, "error": err}).Error("on sending transactions")
		return err
	}

	if len(hosts) > 0 {
		markSent(trs)
	}
	return nil
}

// unsentTransactions are those of the pool not sent to the other nodes yet
func unsentTransactions() []tcpclient.Tx {
	data, hashes := transaction.TxPool.Unsent(syspar.GetMaxTxCount())
	trs := make([]tcpclient.Tx, len(data))
	for i := range data {
		trs[i] = tcpclient.Tx{Hash: hashes[i], Data: data[i]}
	}
	return trs
}

func markSent(trs []tcpclient.Tx) {
	hashes := make([][]byte, len(trs))
	for i, tr := range trs {
		hashes[i] = tr.Hash
	}
	transaction.TxPool.MarkSent(hashes)
}

// send block and transactions hashes
func sendBlockWithTxHashes(ctx context.Context, honorNodeID int64, logger *log.Entry) error {
	bftChain := syspar.IsBftConsensus()
	var block *sqldb.InfoBlock
	var err error
	if !bftChain {
		// a validator of a chain of the BFT consensus announces no block: the others commit it
		// by its certificate
		block, err = sqldb.BlockGetUnsent()
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting unsent blocks")
			return err
		}
	}

	trs := unsentTransactions()
	if len(trs) == 0 && block == nil {
		// it's nothing to send
		logger.Debug("nothing to send")
		return nil
	}
	selectModel := SelectModel{}.GetWorkMode()
	_, ok := selectModel.(*HonorNodeMode)
	var (
		hosts          []string
		banHosts       []string
		candidateNodes sqldb.CandidateNodes
	)
	if bftChain {
		// a chain of the BFT consensus bans no node locally: every validator has all transactions
		hosts = syspar.GetRemoteHosts()
	} else if ok {
		hosts, banHosts, err = node.GetNodesBanService().FilterHosts(syspar.GetRemoteHosts())
	} else {
		candidateNodes, err = GetCandidateNodes()
		for _, node := range candidateNodes {
			hosts = append(hosts, node.TcpAddress)
		}
	}

	if err != nil {
		log.WithError(err).Error("on getting remotes hosts")
		return err
	}
	if len(banHosts) > 0 {
		if err := tcpclient.SendFullBlockToAll(ctx, banHosts, nil, trs, honorNodeID); err != nil {
			log.WithFields(log.Fields{"type": consts.TCPClientError, "error": err}).Warn("on sending block with hashes to ban hosts")
			return err
		}
	}
	if err := tcpclient.SendFullBlockToAll(ctx, hosts, block, trs, honorNodeID); err != nil {
		log.WithFields(log.Fields{"type": consts.TCPClientError, "error": err}).Warn("on sending block with hashes to all")
		return err
	}

	// mark all transactions and block as sent
	if block != nil {
		err = block.MarkSent()
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("marking block sent")
			return err
		}
	}

	markSent(trs)
	return nil
}
