/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package node

import (
	"context"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

var updatingEndWhilePaused = make(chan struct{})

type NodeRelevanceService struct {
	availableBlockchainGap int64
	checkingInterval       time.Duration
}

func NewNodeRelevanceService() *NodeRelevanceService {
	var availableBlockchainGap int64 = consts.AvailableBCGap
	if syspar.GetRbBlocks1() > consts.AvailableBCGap {
		availableBlockchainGap = syspar.GetRbBlocks1() - consts.AvailableBCGap
	}

	checkingInterval := syspar.GetMaxBlockTimeDuration() * time.Duration(syspar.GetRbBlocks1()-consts.DefaultNodesConnectDelay)
	return &NodeRelevanceService{
		availableBlockchainGap: availableBlockchainGap,
		checkingInterval:       checkingInterval,
	}
}

// Run is starting node monitoring
func (n *NodeRelevanceService) Run(ctx context.Context) {
	go func() {
		log.Info("Node relevance monitoring started")
		for {
			relevance, err := n.checkNodeRelevance(ctx)
			if err != nil {
				log.WithFields(log.Fields{"type": consts.BCRelevanceError, "err": err}).Error("checking blockchain relevance")
				return
			}

			if !relevance && !IsNodePaused() {
				log.Info("Node Relevance Service is pausing node activity")
				n.pauseNodeActivity()
			}

			if relevance && IsNodePaused() {
				log.Info("Node Relevance Service is resuming node activity")
				n.resumeNodeActivity()
			}

			select {
			case <-time.After(n.checkingInterval):
			case <-updatingEndWhilePaused:
			}
		}
	}()
}

func NodeDoneUpdatingBlockchain() {
	if IsNodePaused() {
		updatingEndWhilePaused <- struct{}{}
	}
}

func (n *NodeRelevanceService) checkNodeRelevance(ctx context.Context) (relevant bool, err error) {
	curBlock := &sqldb.InfoBlock{}
	_, err = curBlock.Get()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "err": err}).Error("retrieving info block from db")
		return false, errors.Wrapf(err, "retrieving info block from db")
	}
	var (
		tx = &sqldb.Transaction{}
		r  bool
	)
	r, err = tx.GetStopNetwork()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "err": err}).Info("retrieving transaction from db")
		return false, nil
	}
	if r {
		return false, nil
	}
	var (
		remoteHosts []string
	)
	if syspar.IsHonorNodeMode() {
		remoteHosts, err = GetNodesBanService().FilterBannedHosts(syspar.GetRemoteHosts())
	} else {
		candidateNodes, err := sqldb.GetCandidateNode(syspar.SysInt(syspar.NumberNodes))
		if err == nil && len(candidateNodes) > 0 {
			for _, node := range candidateNodes {
				remoteHosts = append(remoteHosts, node.TcpAddress)
			}
		}
	}

	if err != nil {
		return false, err
	}
	// Node is single in blockchain network and it can't be irrelevant
	if len(remoteHosts) == 0 {
		return true, nil
	}

	_, maxBlockID, err := tcpclient.HostWithMaxBlock(ctx, remoteHosts)
	if err != nil {
		if err == tcpclient.ErrNodesUnavailable {
			return false, nil
		}
		return false, errors.Wrapf(err, "choosing best host")
	}

	// Node can't connect to others
	if maxBlockID == -1 {
		log.WithFields(log.Fields{"hosts": remoteHosts}).Info("can't connect to others, stopping node relevance")
		return false, nil
	}

	// Node blockchain is stale
	if curBlock.BlockID+n.availableBlockchainGap < maxBlockID {
		log.WithFields(log.Fields{"maxBlockID": maxBlockID, "curBlockID": curBlock.BlockID, "Gap": n.availableBlockchainGap}).Info("blockchain is stale, stopping node relevance")
		return false, nil
	}

	return true, nil
}

func (n *NodeRelevanceService) pauseNodeActivity() {
	np.Set(PauseTypeUpdatingBlockchain)
}

func (n *NodeRelevanceService) resumeNodeActivity() {
	np.Unset()
}
