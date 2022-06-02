/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
	log "github.com/sirupsen/logrus"
)

func BlockGeneratorCandidate(ctx context.Context, d *daemon) error {
	defer func() {
		d.sleepTime = syspar.GetMaxBlockTimeDuration()
	}()
	if node.IsNodePaused() {
		return nil
	}
	prevBlock := &sqldb.InfoBlock{}
	_, err := prevBlock.Get()
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting previous block")
		return err
	}
	NodePrivateKey, NodePublicKey := utils.GetNodeKeys()
	if len(NodePrivateKey) < 1 {
		d.logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node private key is empty")
		return errors.New(`node private key is empty`)
	}
	if len(NodePublicKey) < 1 {
		d.logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node public key is empty")
		return errors.New(`node public key is empty`)
	}

	var (
		candidateNodes []sqldb.CandidateNode
	)
	candidateNodes, err = sqldb.GetCandidateNode(syspar.SysInt(syspar.NumberNodes))
	if err != nil {
		log.WithError(err).Error("getting candidate node list")
		return err
	}
	currentCandidateNode, nodePosition := GetThisNodePosition(candidateNodes, prevBlock)
	if !nodePosition {
		d.sleepTime = 4 * time.Second
		d.logger.WithFields(log.Fields{"type": consts.JustWaiting, "error": err}).Debug("we are not honor node, sleep for 10 seconds")
		return nil
	}
	st := time.Now()
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

	trs, err := processTransactions(d.logger, txs, st)
	if err != nil {
		return err
	}
	// Block generation will be started only if we have transactions
	if len(trs) == 0 {
		return nil
	}
	candidateNodesByte, _ := json.Marshal(candidateNodes)
	header := &types.BlockHeader{
		BlockId:        prevBlock.BlockID + 1,
		Timestamp:      st.Unix(),
		EcosystemId:    0,
		KeyId:          conf.Config.KeyID,
		NodePosition:   currentCandidateNode.ID,
		Version:        consts.BlockVersion,
		ConsensusMode:  consts.CandidateNodeMode,
		CandidateNodes: candidateNodesByte,
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

	return nil
}

func GetThisNodePosition(candidateNodes []sqldb.CandidateNode, prevBlock *sqldb.InfoBlock) (sqldb.CandidateNode, bool) {
	candidateNode := sqldb.CandidateNode{}
	if len(candidateNodes) == 0 {

		firstBlock, err := syspar.GetFirstBlockData()
		if err != nil {
			return candidateNode, false
		}
		nodePubKey := syspar.GetNodePubKey()
		if bytes.Equal(firstBlock.NodePublicKey, nodePubKey) {
			candidateNode.ID = 0
			candidateNode.NodePubKey = hex.EncodeToString(nodePubKey)
			syspar.SetRunModel(consts.HonorNodeMode)
			return candidateNode, true
		}
		return candidateNode, false
	}

	if len(candidateNodes) == 1 {
		nodePubKey := candidateNodes[0].NodePubKey
		pk, err := hex.DecodeString(nodePubKey)
		if err != nil {
			return candidateNode, false
		}
		pk = crypto.CutPub(pk)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.ConversionError, "error": err}).Error("decoding node private key from hex")
			return candidateNode, false
		}
		if bytes.Equal(pk, syspar.GetNodePubKey()) {
			return candidateNodes[0], true
		}
		return candidateNode, false
	}

	if len(candidateNodes) == 2 {
		_, NodePublicKey := utils.GetNodeKeys()
		NodePublicKey = "04" + NodePublicKey
		var (
			packageNode sqldb.CandidateNode
			flag        bool
		)
		for _, node := range candidateNodes {

			if NodePublicKey == node.NodePubKey && prevBlock.NodePosition != strconv.FormatInt(node.ID, 10) {
				flag = true
				packageNode = node
				break
			}
			packageNode = node
		}
		if flag {
			return packageNode, flag
		}
		return packageNode, flag
	}
	if len(candidateNodes) > 2 {
		candidateNodesSqrt := math.Sqrt(float64(len(candidateNodes)))
		candidateNodesCeil := math.Ceil(candidateNodesSqrt)
		startBlockId := prevBlock.BlockID - int64(candidateNodesCeil)
		subBlocks, err := sqldb.GetBlockchain(startBlockId, prevBlock.BlockID, sqldb.OrderASC)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting recent block")
			return candidateNode, false
		}
		size := len(candidateNodes)
		for i, subBlock := range subBlocks {
			for j := 0; j < size; j++ {
				if candidateNodes[j].ID == subBlock.NodePosition {
					candidateNodes = append(candidateNodes[:j], candidateNodes[j+1:]...)
					size = len(candidateNodes)
					i--
				}
			}
		}
		if len(candidateNodes) > 0 {
			maxVal := candidateNodes[0].ReplyCount
			maxIndex := 0
			for i, node := range candidateNodes {
				if maxVal < node.ReplyCount {
					maxVal = candidateNodes[i].ReplyCount
					maxIndex = i
				}
			}
			_, NodePublicKey := utils.GetNodeKeys()
			if len(NodePublicKey) < 1 {
				log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node public key is empty")
				return candidateNode, false
			}
			NodePublicKey = "04" + NodePublicKey

			if NodePublicKey == candidateNodes[maxIndex].NodePubKey {
				return candidateNodes[maxIndex], true
			}

		}
	}
	return candidateNode, false
}

func checkVotes(candidateNodes int64, votes int64) bool {
	lessVotes := math.Ceil(float64(candidateNodes / 2))
	if votes >= int64(lessVotes) {
		return true
	}
	return false
}
