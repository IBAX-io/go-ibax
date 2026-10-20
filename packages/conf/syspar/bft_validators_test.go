/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package syspar

import (
	"fmt"
	"testing"

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
