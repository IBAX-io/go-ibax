/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sync"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

// The peers of the consensus are the honor nodes and the validators of every height the node may
// still vote on. A block that changes honor_nodes changes the set BftSetDelay blocks later, and
// until then the old set votes: a validator the block removed (or stopped) is no honor node any
// more, yet the heights before the new set need its votes, so the node sends it the proposals,
// the votes and the pending transactions until the new set takes effect. Its address is that of
// honor_nodes before the change, which the rollback records of the change keep.

// bftPeerWindow is the heights the node may still vote on: from the height after the last
// committed block up to the last height whose set it knows
func bftPeerWindow(executed, committed int64) (from, to int64) {
	to = executed + consts.BftSetDelay
	from = min(committed+1, to)
	return max(from, 1), max(to, 1)
}

// windowValidators is the union of the validators sets of the heights from from up to to: the
// set of from and the sets of the heights in between at which the set changes
func windowValidators(from, to int64, setAt func(int64) (bft.Validators, error), changes func(from, to int64) ([]int64, error)) ([][]byte, error) {
	heights := []int64{from}
	if to > from {
		hs, err := changes(from+1, to)
		if err != nil {
			return nil, err
		}
		heights = append(heights, hs...)
	}
	var keys [][]byte
	for _, h := range heights {
		set, err := setAt(h)
		if err != nil {
			return nil, err
		}
		for _, k := range set.Keys {
			if !slices.ContainsFunc(keys, func(x []byte) bool { return bytes.Equal(x, k) }) {
				keys = append(keys, k)
			}
		}
	}
	return keys, nil
}

// peerHosts is the hosts of the honor nodes and of the validators keys, self excluded: the
// address of a key is the current one, or else the latest one of the history (honor_nodes values
// before changes, the latest first). A key without an address is returned in missing.
func peerHosts(remote []string, keys [][]byte, self []byte, current func([]byte) (string, bool), history [][]syspar.HonorNode) (hosts []string, missing [][]byte) {
	hosts = slices.Clone(remote)
	add := func(h string) {
		if h != "" && !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	for _, k := range keys {
		if bytes.Equal(k, self) {
			continue
		}
		if addr, ok := current(k); ok {
			add(addr)
			continue
		}
		addr, ok := historyAddress(history, k)
		if !ok {
			missing = append(missing, k)
			continue
		}
		add(addr)
	}
	return hosts, missing
}

func historyAddress(history [][]syspar.HonorNode, key []byte) (string, bool) {
	for _, nodes := range history {
		for _, n := range nodes {
			if bytes.Equal(n.PublicKey, key) {
				return n.TCPAddress, true
			}
		}
	}
	return "", false
}

// honorNodesHistory is the honor_nodes values before the changes of the blocks from fromBlock on,
// the latest change first
func honorNodesHistory(fromBlock int64) ([][]syspar.HonorNode, error) {
	values, err := sqldb.PlatformParameterHistory(nil, syspar.HonorNodes, fromBlock)
	if err != nil {
		return nil, err
	}
	history := make([][]syspar.HonorNode, 0, len(values))
	for _, v := range values {
		var nodes []syspar.HonorNode
		if err := json.Unmarshal([]byte(v), &nodes); err != nil {
			return nil, err
		}
		history = append(history, nodes)
	}
	return history, nil
}

// bftPeerCache keeps the peers of the last window and honor nodes, which change only with the
// blocks
type bftPeerCache struct {
	mu     sync.Mutex
	from   int64
	to     int64
	remote []string
	hosts  []string
}

var bftPeerList bftPeerCache

// bftPeers is the hosts the node sends the consensus messages to and synchronizes from. On an
// error it keeps the last peers, which have at least the honor nodes.
func bftPeers(logger *log.Entry) []string {
	remote := syspar.GetRemoteHosts()
	executed, committed, err := heights()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("reading the heights for the peers")
		return bftPeerList.last(remote)
	}
	from, to := bftPeerWindow(executed, committed)
	c := &bftPeerList
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hosts != nil && c.from == from && c.to == to && slices.Equal(c.remote, remote) {
		return slices.Clone(c.hosts)
	}
	keys, err := windowValidators(from, to, func(h int64) (bft.Validators, error) { return validatorsAt(uint64(h)) },
		func(from, to int64) ([]int64, error) { return sqldb.SetChanges(nil, from, to) })
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "from": from, "to": to}).Error("reading the validators of the peers")
		return c.lastLocked(remote)
	}
	hosts, missing := peerHosts(remote, keys, syspar.GetNodePubKey(), syspar.GetNodeTCPAddress, nil)
	if len(missing) > 0 {
		// the set of from took effect at most BftSetDelay blocks after the block that changed it
		history, err := honorNodesHistory(from - 2*consts.BftSetDelay)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("reading the former honor nodes")
			return c.lastLocked(remote)
		}
		hosts, missing = peerHosts(remote, keys, syspar.GetNodePubKey(), syspar.GetNodeTCPAddress, history)
		for _, k := range missing {
			logger.WithFields(log.Fields{"type": consts.NotFound, "key": hex.EncodeToString(k)}).Warn("a validator has no address")
		}
	}
	c.from, c.to, c.remote, c.hosts = from, to, remote, hosts
	return slices.Clone(hosts)
}

func (c *bftPeerCache) last(remote []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastLocked(remote)
}

func (c *bftPeerCache) lastLocked(remote []string) []string {
	hosts := slices.Clone(remote)
	for _, h := range c.hosts {
		if !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	return hosts
}
