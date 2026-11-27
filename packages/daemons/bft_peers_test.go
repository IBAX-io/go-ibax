/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"errors"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"

	"github.com/stretchr/testify/require"
)

// TestBftPeerWindow: the window runs from the height after the last committed block up to the
// last height whose set the node knows; a node whose execution is far behind its commits has the
// last known height alone
func TestBftPeerWindow(t *testing.T) {
	for _, c := range []struct{ executed, committed, from, to int64 }{
		{executed: 162, committed: 162, from: 163, to: 162 + consts.BftSetDelay},
		{executed: 160, committed: 162, from: 163, to: 160 + consts.BftSetDelay},
		{executed: 150, committed: 162, from: 150 + consts.BftSetDelay, to: 150 + consts.BftSetDelay},
		{executed: 0, committed: 0, from: 1, to: consts.BftSetDelay},
	} {
		from, to := bftPeerWindow(c.executed, c.committed)
		require.Equal(t, [2]int64{c.from, c.to}, [2]int64{from, to}, "executed %d committed %d", c.executed, c.committed)
	}
}

// testSets is the sets of a chain whose set {1,2,3,5} changes to {1,2,4,6,7} from height 165 (the
// change block of the feedback, 162, plus BftSetDelay)
func testSets() (func(int64) (bft.Validators, error), func(int64, int64) ([]int64, error)) {
	old := bft.Validators{Keys: [][]byte{{1}, {2}, {3}, {5}}}
	next := bft.Validators{Keys: [][]byte{{1}, {2}, {4}, {6}, {7}}}
	setAt := func(h int64) (bft.Validators, error) {
		if h >= 165 {
			return next, nil
		}
		return old, nil
	}
	changes := func(from, to int64) ([]int64, error) {
		if from <= 165 && 165 <= to {
			return []int64{165}, nil
		}
		return nil, nil
	}
	return setAt, changes
}

// TestWindowValidators: in the heights before a new set takes effect the peers are the old and the
// new validators, so the removed ones go on getting the messages they vote on; once the new set
// took effect they are the new set alone
func TestWindowValidators(t *testing.T) {
	setAt, changes := testSets()
	keys, err := windowValidators(163, 165, setAt, changes)
	require.NoError(t, err)
	require.Equal(t, [][]byte{{1}, {2}, {3}, {5}, {4}, {6}, {7}}, keys)
	keys, err = windowValidators(166, 168, setAt, changes)
	require.NoError(t, err)
	require.Equal(t, [][]byte{{1}, {2}, {4}, {6}, {7}}, keys)
	keys, err = windowValidators(165, 165, setAt, changes)
	require.NoError(t, err)
	require.Equal(t, [][]byte{{1}, {2}, {4}, {6}, {7}}, keys)

	// the set of the first height is the set of the window even when no set changes in it
	keys, err = windowValidators(160, 162, setAt, func(int64, int64) ([]int64, error) { return nil, nil })
	require.NoError(t, err)
	require.Equal(t, [][]byte{{1}, {2}, {3}, {5}}, keys)

	fail := errors.New("db")
	_, err = windowValidators(163, 165, setAt, func(int64, int64) ([]int64, error) { return nil, fail })
	require.ErrorIs(t, err, fail)
	_, err = windowValidators(163, 165, func(int64) (bft.Validators, error) { return bft.Validators{}, fail }, changes)
	require.ErrorIs(t, err, fail)
}

// TestPeerHosts: the hosts are the honor nodes and the validators of the window but this node; a
// validator no honor node any more has its latest former address, and one with none is missing
func TestPeerHosts(t *testing.T) {
	current := map[byte]string{1: "h1", 2: "h2", 4: "h4", 6: "h6", 7: "h7"}
	lookup := func(k []byte) (string, bool) { a, ok := current[k[0]]; return a, ok }
	remote := []string{"h2", "h4", "h6", "h7"}
	keys := [][]byte{{1}, {2}, {3}, {5}, {4}, {6}, {7}}
	history := [][]syspar.HonorNode{
		{{PublicKey: []byte{3}, TCPAddress: "h3new"}, {PublicKey: []byte{5}, TCPAddress: "h5"}},
		{{PublicKey: []byte{3}, TCPAddress: "h3old"}},
	}
	hosts, missing := peerHosts(remote, keys, []byte{1}, lookup, history)
	require.Equal(t, []string{"h2", "h4", "h6", "h7", "h3new", "h5"}, hosts)
	require.Empty(t, missing)

	hosts, missing = peerHosts(remote, keys, []byte{1}, lookup, nil)
	require.Equal(t, []string{"h2", "h4", "h6", "h7"}, hosts)
	require.Equal(t, [][]byte{{3}, {5}}, missing)

	// a stopped validator is no remote host but has its address while it is in the window
	hosts, _ = peerHosts([]string{"h2"}, [][]byte{{1}, {2}, {4}}, []byte{1}, lookup, nil)
	require.Equal(t, []string{"h2", "h4"}, hosts)
}

// TestPeerCacheLast: on an error the node keeps the last peers besides the honor nodes
func TestPeerCacheLast(t *testing.T) {
	c := &bftPeerCache{hosts: []string{"h2", "h3"}}
	require.Equal(t, []string{"h2", "h4", "h3"}, c.last([]string{"h2", "h4"}))
	require.Equal(t, []string{"h4"}, (&bftPeerCache{}).last([]string{"h4"}))
}
