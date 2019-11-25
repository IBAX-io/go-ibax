/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package service

import "sync"

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

func (np *NodePaused) Set(pt PauseType) {
	np.mutex.Lock()
	defer np.mutex.Unlock()

	np.PauseType = pt
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
