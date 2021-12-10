/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package node

import (
	"sync"
)

const (
	NoPause PauseType = 0

	PauseTypeUpdatingBlockchain PauseType = 1 + iota
	PauseTypeStopingNetwork
)

// np contains the reason why a node should not generating blocks
var np = &NodePaused{PauseType: NoPause}

type PauseType int

type NodePaused struct {
	mutex sync.RWMutex

	PauseType PauseType
}

func (p PauseType) String() string {
	switch p {
	case NoPause:
		return "node server status is running"
	case PauseTypeUpdatingBlockchain:
		return "node server is updating"
	case PauseTypeStopingNetwork:
		return "node server is stopped"
	}
	return "node server is unknown"
}

func (np *NodePaused) Set(pt PauseType) {
	np.mutex.Lock()
	defer np.mutex.Unlock()

	np.PauseType = pt
}

func (np *NodePaused) Unset() {
	np.mutex.Lock()
	defer np.mutex.Unlock()

	np.PauseType = NoPause
}

func (np *NodePaused) Get() PauseType {
	np.mutex.RLock()
	defer np.mutex.RUnlock()

	return np.PauseType
}

func (np *NodePaused) IsSet() bool {
	np.mutex.RLock()
	defer np.mutex.RUnlock()

	return np.PauseType != NoPause
}

func IsNodePaused() bool {
	return np.IsSet()
}

func PauseNodeActivity(pt PauseType) {
	np.Set(pt)
}

func NodePauseType() PauseType {
	return np.Get()
}
