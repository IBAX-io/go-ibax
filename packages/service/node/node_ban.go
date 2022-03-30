/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package node

import (
	"bytes"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type localBannedNode struct {
	HonorNode      *syspar.HonorNode
	LocalUnBanTime time.Time
}

type NodesBanService struct {
	localBannedNodes map[int64]localBannedNode
	honorNodes       []syspar.HonorNode

	m *sync.Mutex
}

var nbs *NodesBanService

// GetNodesBanService is returning nodes ban service
func GetNodesBanService() *NodesBanService {
	return nbs
}

// InitNodesBanService initializing nodes ban storage
func InitNodesBanService() error {
	nbs = &NodesBanService{
		localBannedNodes: make(map[int64]localBannedNode),
		m:                &sync.Mutex{},
	}

	nbs.refreshNodes()
	return nil
}

// RegisterBadBlock is set node to local ban and saving bad block to global registry
func (nbs *NodesBanService) RegisterBadBlock(node syspar.HonorNode, badBlockId, blockTime int64, reason string, register bool) error {
	if nbs.IsBanned(node) {
		return nil
	}

	nbs.localBan(node)
	if !register {
		return nil
	}
	err := nbs.newBadBlock(node, badBlockId, blockTime, reason)
	if err != nil {
		return err
	}

	return nil
}

// IsBanned is allows to check node ban (local or global)
func (nbs *NodesBanService) IsBanned(node syspar.HonorNode) bool {
	nbs.refreshNodes()

	nbs.m.Lock()
	defer nbs.m.Unlock()

	nodeKeyID := crypto.Address(node.PublicKey)
	// Searching for local ban
	now := time.Now()

	if fn, ok := nbs.localBannedNodes[nodeKeyID]; ok {
		if now.Equal(fn.LocalUnBanTime) || now.After(fn.LocalUnBanTime) {
			delete(nbs.localBannedNodes, nodeKeyID)
			return false
		}

		return true
	}

	// Searching for global ban.
	// Here we don't estimating global ban expiration. If ban time doesn't equal zero - we assuming
	// that node is still banned (even if `unban` time has already passed)
	for _, fn := range nbs.honorNodes {
		if bytes.Equal(fn.PublicKey, node.PublicKey) {
			if !fn.UnbanTime.Equal(time.Unix(0, 0)) {
				return true
			} else {
				break
			}
		}
	}

	return false
}

func (nbs *NodesBanService) refreshNodes() {
	nbs.m.Lock()
	nbs.honorNodes = syspar.GetNodes()
	nbs.m.Unlock()
}

func (nbs *NodesBanService) localBan(node syspar.HonorNode) {
	nbs.m.Lock()
	defer nbs.m.Unlock()

	ts := time.Now().Unix()
	te := time.Now().Add(syspar.GetLocalNodeBanTime()).Unix()
	if te == ts {
		te = ts + 120
	}
	nbs.localBannedNodes[crypto.Address(node.PublicKey)] = localBannedNode{
		HonorNode:      &node,
		LocalUnBanTime: time.Unix(te, 0),
		//LocalUnBanTime: time.Now().Add(syspar.GetLocalNodeBanTime()),
	}
}

func (nbs *NodesBanService) newBadBlock(producer syspar.HonorNode, blockId, blockTime int64, reason string) error {
	var currentNode syspar.HonorNode
	nbs.m.Lock()
	for _, fn := range nbs.honorNodes {
		if bytes.Equal(fn.PublicKey, syspar.GetNodePubKey()) {
			currentNode = fn
			break
		}
	}
	nbs.m.Unlock()

	if len(currentNode.PublicKey) == 0 {
		return errors.New("cant find current node in honor nodes list")
	}

	vm := script.GetVM()
	contract := smart.VMGetContract(vm, "NewBadBlock", 1)
	info := contract.Info()

	sc := types.SmartTransaction{
		Header: &types.Header{
			ID:          int(info.ID),
			EcosystemID: 1,
			Time:        time.Now().Unix(),
			KeyID:       conf.Config.KeyID,
		},
		Params: map[string]any{
			"ProducerNodeID": crypto.Address(producer.PublicKey),
			"ConsumerNodeID": crypto.Address(currentNode.PublicKey),
			"BlockID":        blockId,
			"Timestamp":      blockTime,
			"Reason":         reason,
		},
	}

	stp := &transaction.SmartTransactionParser{
		SmartContract: &smart.SmartContract{TxSmart: new(types.SmartTransaction)},
	}
	txData, err := stp.BinMarshalWithPrivate(&sc, syspar.GetNodePrivKey(), true)
	if err != nil {
		return err
	}
	return transaction.CreateTransaction(txData, stp.Hash, conf.Config.KeyID, stp.Timestamp)
}

func (nbs *NodesBanService) FilterHosts(hosts []string) ([]string, []string, error) {
	var goodHosts, banHosts []string
	for _, h := range hosts {
		n, err := syspar.GetNodeByHost(h)
		if err != nil {
			log.WithFields(log.Fields{"error": err, "host": h}).Error("getting node by host")
			return nil, nil, err
		}

		if nbs.IsBanned(n) {
			banHosts = append(banHosts, n.TCPAddress)
		} else {
			goodHosts = append(goodHosts, n.TCPAddress)
		}
	}
	return goodHosts, banHosts, nil
}

func (nbs *NodesBanService) FilterBannedHosts(hosts []string) (goodHosts []string, err error) {
	goodHosts, _, err = nbs.FilterHosts(hosts)
	return
}
