/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package syspar

import (
	"fmt"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

func bftTestNodes(t *testing.T, n int) []*HonorNode {
	t.Helper()
	nodes := make([]*HonorNode, n)
	for i := range nodes {
		_, pub, err := crypto.GenNodeKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		nodes[i] = &HonorNode{
			TCPAddress: fmt.Sprintf("127.0.0.1:%d", 7078+i),
			APIAddress: fmt.Sprintf("http://127.0.0.1:%d", 7079+i),
			PublicKey:  pub,
		}
	}
	return nodes
}

func TestCheckBftValidators(t *testing.T) {
	crypto.InitAsymAlgo("ECC_P256")
	if err := CheckBftValidators(bftTestNodes(t, MinBftValidators)); err != nil {
		t.Fatalf("%d validators: %v", MinBftValidators, err)
	}
	if err := CheckBftValidators(bftTestNodes(t, MinBftValidators-1)); err == nil {
		t.Fatalf("%d validators pass", MinBftValidators-1)
	}
	stopped := bftTestNodes(t, MinBftValidators+1)
	stopped[1].Stopped = true
	if err := CheckBftValidators(stopped); err != nil {
		t.Fatalf("%d validators with one stopped: %v", MinBftValidators+1, err)
	}
	stopped[2].Stopped = true
	if err := CheckBftValidators(stopped); err == nil {
		t.Fatalf("%d validators not stopped pass", MinBftValidators-1)
	}
	dup := bftTestNodes(t, MinBftValidators)
	dup[3].PublicKey = dup[0].PublicKey
	if err := CheckBftValidators(dup); err != errHonorNodeDuplicatePublicKey {
		t.Fatalf("a duplicate key: %v", err)
	}
	bad := bftTestNodes(t, MinBftValidators)
	bad[2].TCPAddress = ""
	if err := CheckBftValidators(bad); err != errHonorNodeInvalidValues {
		t.Fatalf("an invalid node: %v", err)
	}
}

// bft_timeout_propose_ms stays above max_block_generation_time and BftProposerWait, and
// bft_stop_review_days above bft_review_voting_days, the starting values too; the bound behind of
// the timeliness grows with the round
func TestCheckBftParameter(t *testing.T) {
	mutex.Lock()
	saved := cache
	cache = map[string]string{MaxBlockGenerationTime: "2000", BftTimeoutPropose: "4000",
		BftStopReviewDays: "7", BftReviewVotingDays: "3"}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		cache = saved
		mutex.Unlock()
	}()
	for _, tc := range []struct {
		name, value string
		ok          bool
	}{
		{BftTimeoutPropose, "4000", true},
		{BftTimeoutPropose, "3001", true},
		{BftTimeoutPropose, "3000", false},
		{BftTimeoutPropose, "0", false},
		{BftTimeoutPropose, "x", false},
		{MaxBlockGenerationTime, "2999", true},
		{MaxBlockGenerationTime, "3000", false},
		{MaxBlockGenerationTime, "-1", false},
		{BftStopReviewDays, "7", true},
		{BftStopReviewDays, "4", true},
		{BftStopReviewDays, "3", false},
		{BftStopReviewDays, "0", false},
		{BftReviewVotingDays, "6", true},
		{BftReviewVotingDays, "7", false},
		{BftReviewVotingDays, "-3", false},
		{BftMaxClockDrift, "x", true}, // no pair: the checks of the parameter apply
	} {
		if err := CheckBftParameter(tc.name, tc.value); (err == nil) != tc.ok {
			t.Errorf("%s %s: %v", tc.name, tc.value, err)
		}
	}
	for _, tc := range []struct {
		round  uint32
		behind time.Duration
	}{{0, 5 * time.Second}, {1, 5500 * time.Millisecond}, {2, 6050 * time.Millisecond}, {100, 5 * time.Second * 13780}} {
		ahead, behind := GetBftTimeliness(tc.round)
		if ahead != 2*time.Second || behind < tc.behind || behind > tc.behind+tc.behind/1000 {
			t.Errorf("round %d: the bounds %v ahead and %v behind, want 2s and %v", tc.round, ahead, behind, tc.behind)
		}
	}
	if _, behind := GetBftTimeliness(1000); behind != func() time.Duration { _, b := GetBftTimeliness(100); return b }() {
		t.Errorf("the bound behind grows past round %d", bftTimelinessRounds)
	}
}

// A vote takes no validator out of a set it leaves below MinBftValidators; it adds validators, and
// removes a node an evidence put out, whatever the size of the set
func TestCheckBftValidatorsChange(t *testing.T) {
	crypto.InitAsymAlgo("ECC_P256")
	nodes := bftTestNodes(t, MinBftValidators+2)
	var outKeys [][]byte
	out := func(publicKey []byte) bool {
		for _, k := range outKeys {
			if string(k) == string(publicKey) {
				return true
			}
		}
		return false
	}
	without := func(nodes []*HonorNode, i int) []*HonorNode {
		return append(append([]*HonorNode{}, nodes[:i]...), nodes[i+1:]...)
	}
	stop := func(nodes []*HonorNode, i int) []*HonorNode {
		next := make([]*HonorNode, len(nodes))
		for j, n := range nodes {
			c := *n
			next[j] = &c
		}
		next[i].Stopped = true
		return next
	}
	five := nodes[:MinBftValidators+1]
	four := nodes[:MinBftValidators]
	for _, tc := range []struct {
		what       string
		prev, next []*HonorNode
		out        [][]byte
		ok         bool
	}{
		{"5 to 4 by a removal", five, without(five, 0), nil, true},
		{"4 to 3 by a removal", four, without(four, 0), nil, false},
		{"4 to 3 by a stop", four, stop(four, 1), nil, false},
		{"5 to 4 by a stop", five, stop(five, 1), nil, true},
		{"3 to 4 by an addition", without(four, 0), four, nil, true},
		{"2 to 3 by an addition", four[:2], four[:3], nil, true},
		{"3 out of 4 with a node out, the node removed", four, without(four, 0), [][]byte{four[0].PublicKey}, true},
		{"3 out of 4 with a node out, another removed", four, without(four, 1), [][]byte{four[0].PublicKey}, false},
		{"a stopped node removed from a set of 3", stop(four, 2), without(four, 2), nil, true},
		{"a duplicate added", four, append(append([]*HonorNode{}, four...), four[0]), nil, false},
	} {
		outKeys = tc.out
		if err := CheckBftValidatorsChange(tc.prev, tc.next, out); (err == nil) != tc.ok {
			t.Errorf("%s: %v", tc.what, err)
		}
	}
}
