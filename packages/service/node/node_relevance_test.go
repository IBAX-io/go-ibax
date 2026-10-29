/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package node

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// closedHosts are addresses of the local host that nothing listens on
func closedHosts(t *testing.T, count int) []string {
	t.Helper()
	hosts := make([]string, count)
	for i := range hosts {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		hosts[i] = l.Addr().String()
		l.Close()
	}
	return hosts
}

func TestRelevanceOf(t *testing.T) {
	n := &NodeRelevanceService{availableBlockchainGap: 10}
	hosts := []string{"127.0.0.1:1"}
	cases := []struct {
		name     string
		bft      bool
		cur, max int64
		err      error
		relevant bool
		fails    bool
	}{
		{name: "bft: none of the others answers", bft: true, err: tcpclient.ErrNodesUnavailable, relevant: true},
		{name: "dpoa: none of the others answers", err: tcpclient.ErrNodesUnavailable},
		{name: "bft: no block of the others", bft: true, max: -1, relevant: true},
		{name: "dpoa: no block of the others", max: -1},
		{name: "bft: the others failed otherwise", bft: true, err: errors.New("context canceled"), fails: true},
		{name: "dpoa: the others failed otherwise", err: errors.New("context canceled"), fails: true},
		{name: "bft: behind past the gap", bft: true, cur: 100, max: 111},
		{name: "dpoa: behind past the gap", cur: 100, max: 111},
		{name: "bft: behind within the gap", bft: true, cur: 100, max: 110, relevant: true},
		{name: "dpoa: behind within the gap", cur: 100, max: 110, relevant: true},
		{name: "bft: ahead", bft: true, cur: 120, max: 110, relevant: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var bft uint64
			if c.bft {
				bft = 1
			}
			syspar.SetFirstBlockData(&types.FirstBlock{BftConsensus: bft})
			relevant, err := n.relevanceOf(c.cur, c.max, c.err, hosts)
			if (err != nil) != c.fails {
				t.Fatalf("error %v, want an error %v", err, c.fails)
			}
			if relevant != c.relevant {
				t.Fatalf("relevant %v, want %v", relevant, c.relevant)
			}
		})
	}
}

// TestRelevanceOfUnreachable asks validators that do not listen yet, as when the validators of a
// chain start together: a validator of the BFT consensus stays relevant, a node of DPoA pauses
func TestRelevanceOfUnreachable(t *testing.T) {
	n := &NodeRelevanceService{availableBlockchainGap: 10}
	hosts := closedHosts(t, 3)
	_, max, err := tcpclient.HostWithMaxBlock(context.Background(), hosts)
	if err != tcpclient.ErrNodesUnavailable {
		t.Fatalf("asking hosts that do not listen: %v, want %v", err, tcpclient.ErrNodesUnavailable)
	}
	for _, c := range []struct {
		bft      uint64
		relevant bool
	}{{1, true}, {0, false}} {
		syspar.SetFirstBlockData(&types.FirstBlock{BftConsensus: c.bft})
		relevant, rerr := n.relevanceOf(0, max, err, hosts)
		if rerr != nil || relevant != c.relevant {
			t.Fatalf("bft %d: relevant %v, error %v; want %v", c.bft, relevant, rerr, c.relevant)
		}
	}
}
