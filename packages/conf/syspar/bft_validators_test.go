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

// bft_timeout_propose_ms stays above max_block_generation_time, the starting values too
func TestCheckBftParameter(t *testing.T) {
	mutex.Lock()
	saved := cache
	cache = map[string]string{MaxBlockGenerationTime: "2000", BftTimeoutPropose: "3000"}
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
		{BftTimeoutPropose, "3000", true},
		{BftTimeoutPropose, "2001", true},
		{BftTimeoutPropose, "2000", false},
		{BftTimeoutPropose, "0", false},
		{BftTimeoutPropose, "x", false},
		{MaxBlockGenerationTime, "2999", true},
		{MaxBlockGenerationTime, "3000", false},
		{MaxBlockGenerationTime, "-1", false},
		{BftMaxClockDrift, "x", true}, // no pair: the checks of the parameter apply
	} {
		if err := CheckBftParameter(tc.name, tc.value); (err == nil) != tc.ok {
			t.Errorf("%s %s: %v", tc.name, tc.value, err)
		}
	}
	early, late := GetBftTimeliness()
	if early != 2*time.Second || late != 5*time.Second {
		t.Errorf("the bounds %v and %v, want 2s and 5s", early, late)
	}
}
