/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/types"
)

type bftTestNode struct {
	TCPAddress string `json:"tcp_address"`
	APIAddress string `json:"api_address"`
	PublicKey  string `json:"public_key"`
	Stopped    bool   `json:"stopped,omitempty"`
}

// bftTestFirstBlock returns the first block of a chain of the BFT consensus with n validators and
// the nodes it names, the first of them the node that generates the block
func bftTestFirstBlock(t *testing.T, n int) (*types.FirstBlock, []bftTestNode) {
	t.Helper()
	nodes := make([]bftTestNode, n)
	var first []byte
	for i := range nodes {
		_, pub, err := crypto.GenNodeKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = pub
		}
		nodes[i] = bftTestNode{
			TCPAddress: fmt.Sprintf("127.0.0.1:%d", 7078+i),
			APIAddress: fmt.Sprintf("http://127.0.0.1:%d", 7079+i),
			PublicKey:  crypto.PubToHex(pub),
		}
	}
	return &types.FirstBlock{NodePublicKey: first, HonorNodesOnly: 1, BftConsensus: 1}, nodes
}

func bftTestSetNodes(t *testing.T, fb *types.FirstBlock, nodes []bftTestNode) {
	t.Helper()
	data, err := json.Marshal(nodes)
	if err != nil {
		t.Fatal(err)
	}
	fb.HonorNodes = string(data)
}

func TestCheckFirstBlockConsensus(t *testing.T) {
	crypto.InitAsymAlgo("ECC_P256")

	if err := CheckFirstBlockConsensus(&types.FirstBlock{}); err != nil {
		t.Fatalf("a chain without the flag: %v", err)
	}
	if err := CheckFirstBlockConsensus(&types.FirstBlock{HonorNodes: "[]"}); err == nil {
		t.Fatal("honor nodes without the flag pass")
	}
	if err := CheckFirstBlockConsensus(&types.FirstBlock{BftConsensus: 2}); err == nil {
		t.Fatal("bft_consensus 2 passes")
	}

	fb, nodes := bftTestFirstBlock(t, 4)
	bftTestSetNodes(t, fb, nodes)
	if err := CheckFirstBlockConsensus(fb); err != nil {
		t.Fatalf("4 validators: %v", err)
	}

	cases := []struct {
		name   string
		change func(fb *types.FirstBlock, nodes []bftTestNode) []bftTestNode
		want   string
	}{
		{"not honor nodes only", func(fb *types.FirstBlock, nodes []bftTestNode) []bftTestNode {
			fb.HonorNodesOnly = 0
			return nodes
		}, "honor nodes only"},
		{"3 validators", func(fb *types.FirstBlock, nodes []bftTestNode) []bftTestNode {
			return nodes[:3]
		}, "at least 4"},
		{"a stopped validator of 4", func(fb *types.FirstBlock, nodes []bftTestNode) []bftTestNode {
			nodes[2].Stopped = true
			return nodes
		}, "at least 4"},
		{"a duplicate key", func(fb *types.FirstBlock, nodes []bftTestNode) []bftTestNode {
			nodes[3].PublicKey = nodes[1].PublicKey
			return nodes
		}, "duplicate publicKey"},
		{"another node first", func(fb *types.FirstBlock, nodes []bftTestNode) []bftTestNode {
			nodes[0], nodes[1] = nodes[1], nodes[0]
			return nodes
		}, "the node that generates it"},
		{"the generating node stopped", func(fb *types.FirstBlock, nodes []bftTestNode) []bftTestNode {
			nodes = append(nodes, bftTestNode{TCPAddress: "127.0.0.1:7100", APIAddress: "http://127.0.0.1:7101", PublicKey: nodes[0].PublicKey})
			_, pub, _ := crypto.GenNodeKeyPair()
			nodes[4].PublicKey = crypto.PubToHex(pub)
			nodes[0].Stopped = true
			return nodes
		}, "the node that generates it"},
	}
	for _, c := range cases {
		fb, nodes := bftTestFirstBlock(t, 4)
		bftTestSetNodes(t, fb, c.change(fb, nodes))
		err := CheckFirstBlockConsensus(fb)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want an error with %q", c.name, err, c.want)
		}
	}

	fb, _ = bftTestFirstBlock(t, 4)
	fb.HonorNodes = "[{]"
	if err := CheckFirstBlockConsensus(fb); err == nil {
		t.Fatal("honor nodes of invalid JSON pass")
	}
}
