/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// sizeNode answers sendTx as a node of the default body size: it refuses a body over sendBodyMax,
// and from its request failAt on (if above 0), every request
type sizeNode struct {
	mu       sync.Mutex
	failAt   int
	requests int
	parts    map[string]bool
}

func (f *sizeNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	if f.failAt > 0 && f.requests >= f.failAt {
		http.Error(w, `{"error":"E_SERVER","msg":"refused"}`, http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, sendBodyMax)
	if err := r.ParseMultipartForm(100000); err != nil {
		http.Error(w, `{"error":"E_SERVER","msg":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	for h := range r.MultipartForm.Value {
		f.parts[h] = true
	}
	w.Write([]byte(`{}`))
}

// txsOf are n transactions of the size of a transfer of an ML-DSA-65 account, its key and its
// signature in the transaction
func txsOf(n int) []*signedTx {
	txs := make([]*signedTx, n)
	for i := range txs {
		txs[i] = &signedTx{hash: fmt.Sprintf("%064x", i), data: make([]byte, 5400)}
	}
	return txs
}

// send splits the transactions into requests under the body size of the node, and sends each
func TestSendSplitsByBodySize(t *testing.T) {
	f := &sizeNode{parts: map[string]bool{}}
	s := httptest.NewServer(f)
	t.Cleanup(s.Close)
	txs := txsOf(600)
	sent, err := newNode(s.URL).send(txs)
	if err != nil {
		t.Fatal(err)
	}
	if sent != len(txs) || len(f.parts) != len(txs) {
		t.Fatalf("sent %d, the node has %d, of %d", sent, len(f.parts), len(txs))
	}
	// 600 × 10.9 kB in hex: 7 requests of 1 MiB at least
	if f.requests < 7 || f.requests > 8 {
		t.Fatalf("%d requests", f.requests)
	}
}

// send tells how many it sent before the request that failed
func TestSendTellsSentBeforeFailure(t *testing.T) {
	f := &sizeNode{failAt: 3, parts: map[string]bool{}}
	s := httptest.NewServer(f)
	t.Cleanup(s.Close)
	sent, err := newNode(s.URL).send(txsOf(600))
	if err == nil {
		t.Fatal("no error")
	}
	if sent == 0 || sent != len(f.parts) {
		t.Fatalf("sent %d, the node has %d", sent, len(f.parts))
	}
}
