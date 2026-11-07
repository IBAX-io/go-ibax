/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/transaction"
)

// fakeNode is a node that makes a block at a request but a send while its pool holds
// transactions, as a validator makes none with an empty pool. Its first send finds the pool full
// and "room" first finds the account with MaxPerKey in it: the pool holds others then, which the
// next block takes; so does every "room..." at its first send. It refuses "bad" for good, and
// puts every other transaction it takes in its next block. A request of more than maxParts
// transactions it refuses as the API, whose multipart reader takes at most 1000 parts.
type fakeNode struct {
	mu       sync.Mutex
	block    int64
	sends    int
	maxParts int
	pool     map[string]bool
	refused  map[string]*txError
	in       map[string]int64
}

func newFakeNode() *fakeNode {
	return &fakeNode{pool: map[string]bool{}, refused: map[string]*txError{}, in: map[string]int64{}}
}

func (f *fakeNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/")
	if path != "sendTx" && len(f.pool) > 0 {
		f.block++
		for h := range f.pool {
			f.in[h] = f.block
			delete(f.pool, h)
		}
	}
	switch path {
	case "sendTx":
		f.sends++
		if f.sends == 1 {
			f.pool["held"] = true
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(apiError{Code: "E_SERVER", Msg: transaction.ErrPoolFull.Error()})
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if f.maxParts > 0 && len(r.MultipartForm.Value) > f.maxParts {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(apiError{Code: "E_SERVER", Msg: "multipart: message too large"})
			return
		}
		for h := range r.MultipartForm.Value {
			switch {
			case h == "bad":
				f.refused[h] = &txError{Type: "txError", Msg: "bad"}
			case strings.HasPrefix(h, "room") && f.refused[h] == nil:
				f.pool["held"] = true
				f.refused[h] = &txError{Type: "txError", Msg: transaction.ErrPoolKeyFull.Error(), Retryable: true}
			default:
				delete(f.refused, h)
				f.pool[h] = true
			}
		}
		json.NewEncoder(w).Encode(map[string]any{})
	case "maxblockid":
		json.NewEncoder(w).Encode(map[string]int64{"max_block_id": f.block})
	case "txstatus":
		var req struct {
			Hashes []string `json:"hashes"`
		}
		json.Unmarshal([]byte(r.FormValue("data")), &req)
		out := map[string]*txStatus{}
		for _, h := range req.Hashes {
			switch {
			case f.in[h] > 0:
				out[h] = &txStatus{BlockID: fmt.Sprint(f.in[h]), Result: "ok " + h}
			case f.refused[h] != nil:
				out[h] = &txStatus{Message: f.refused[h]}
			case f.pool[h]:
				out[h] = &txStatus{}
			default:
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(apiError{Code: "E_HASHNOTFOUND", Msg: h})
				return
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"results": out})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// await sends a batch the pool has no room for again after a block, asks each node of the
// transactions sent to it, sends again one a pool refused for its room and fails one refused
// otherwise: after the block past the one they were sent at, which comes, though the node may
// have made it before the bench saw the refusal and make none after
func TestAwaitSendsAgainWhatAPoolHadNoRoomFor(t *testing.T) {
	fakes := []*fakeNode{newFakeNode(), newFakeNode()}
	c := &chainClient{}
	var outs []*outbox
	for _, f := range fakes {
		s := httptest.NewServer(f)
		t.Cleanup(s.Close)
		n := newNode(s.URL)
		c.nodes = append(c.nodes, n)
		outs = append(outs, newOutbox(n, 2))
	}
	tx := func(h string) *signedTx { return &signedTx{hash: h, data: []byte(h)} }
	outs[0].queue = [][]*signedTx{{tx("a"), tx("b")}}
	outs[1].queue = [][]*signedTx{{tx("c"), tx("room")}, {tx("bad")}}

	results, failed, pending, err := c.await(outs, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending %v", pending)
	}
	want := map[string]string{"a": "ok a", "b": "ok b", "c": "ok c", "room": "ok room"}
	if fmt.Sprint(results) != fmt.Sprint(want) {
		t.Fatalf("results %v", results)
	}
	if len(failed) != 1 || failed["bad"] != "txError: bad" {
		t.Fatalf("failed %v", failed)
	}
	if outs[0].resent != 0 || outs[1].resent != 1 {
		t.Fatalf("sent again %d %d", outs[0].resent, outs[1].resent)
	}
	// each node had its own transactions only
	if fakes[0].in["c"] != 0 || fakes[1].in["a"] != 0 {
		t.Fatal("a transaction went to another node")
	}
}

// await sends again, batch at a time, the transactions a pool refused for its room: more than a
// request of the API takes at once, as when one key sends more than the MaxPerKey of the pool
func TestAwaitSendsAgainBatchAtATime(t *testing.T) {
	f := newFakeNode()
	f.maxParts = 10
	s := httptest.NewServer(f)
	t.Cleanup(s.Close)
	n := newNode(s.URL)
	c := &chainClient{nodes: []*node{n}}
	o := newOutbox(n, 10)
	for i := 0; i < 25; i += 10 {
		var b []*signedTx
		for j := i; j < min(i+10, 25); j++ {
			h := fmt.Sprintf("room%d", j)
			b = append(b, &signedTx{hash: h, data: []byte(h)})
		}
		o.queue = append(o.queue, b)
	}

	results, failed, pending, err := c.await([]*outbox{o}, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 25 || len(failed) != 0 || len(pending) != 0 {
		t.Fatalf("results %d, failed %v, pending %v", len(results), failed, pending)
	}
	if o.resent != 25 {
		t.Fatalf("sent again %d", o.resent)
	}
}
