/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"sync"
	"time"
)

// watcher reads the new blocks of each node as they come and notes when it first saw each
// transaction in a block of the node: a transaction is in a block at the first node that has it,
// and queryable everywhere at the last
type watcher struct {
	nodes []*node
	poll  time.Duration

	mu     sync.Mutex
	seen   []map[string]time.Time // by node, the time each transaction was seen
	blocks []blockSeen            // the blocks of the first node, in order
	errs   int

	stop chan struct{}
	done sync.WaitGroup
}

// blockSeen is a block of the first node: when it was seen and how many transactions it has
type blockSeen struct {
	id  int64
	at  time.Time
	txs int
}

// startWatcher watches the blocks after the last block each node has now
func startWatcher(nodes []*node, poll time.Duration) (*watcher, error) {
	w := &watcher{nodes: nodes, poll: poll, stop: make(chan struct{})}
	for i, n := range nodes {
		last, err := n.maxBlock()
		if err != nil {
			return nil, err
		}
		w.seen = append(w.seen, map[string]time.Time{})
		w.done.Add(1)
		go w.watch(i, last)
	}
	return w, nil
}

func (w *watcher) watch(i int, last int64) {
	defer w.done.Done()
	n := w.nodes[i]
	tick := time.NewTicker(w.poll)
	defer tick.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-tick.C:
		}
		top, err := n.maxBlock()
		if err != nil {
			w.fail()
			continue
		}
		for last < top {
			to := min(top, last+100)
			blocks, err := n.blockTxs(last+1, to)
			if err != nil {
				w.fail()
				break
			}
			now := time.Now()
			w.mu.Lock()
			for id := last + 1; id <= to; id++ {
				for _, h := range blocks[id] {
					if _, ok := w.seen[i][h]; !ok {
						w.seen[i][h] = now
					}
				}
				if i == 0 {
					w.blocks = append(w.blocks, blockSeen{id: id, at: now, txs: len(blocks[id])})
				}
			}
			w.mu.Unlock()
			last = to
		}
	}
}

func (w *watcher) fail() {
	w.mu.Lock()
	w.errs++
	w.mu.Unlock()
}

// close stops the watcher
func (w *watcher) close() {
	close(w.stop)
	w.done.Wait()
}

// inBlock is when the first node had the transaction in a block, and when the last did; ok is
// whether one did
func (w *watcher) inBlock(hash string) (first, all time.Time, inFirst, inAll bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	inAll = true
	for _, seen := range w.seen {
		at, ok := seen[hash]
		if !ok {
			inAll = false
			continue
		}
		if !inFirst || at.Before(first) {
			first = at
		}
		inFirst = true
		if at.After(all) {
			all = at
		}
	}
	return first, all, inFirst, inAll && inFirst
}

// count is how many of the hashes some node, and all nodes, have in a block
func (w *watcher) count(hashes []string) (some, all int) {
	for _, h := range hashes {
		_, _, s, a := w.inBlock(h)
		if s {
			some++
		}
		if a {
			all++
		}
	}
	return
}

// blocksBetween are the blocks of the first node seen between two times
func (w *watcher) blocksBetween(from, to time.Time) []blockSeen {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []blockSeen
	for _, b := range w.blocks {
		if !b.at.Before(from) && !b.at.After(to) {
			out = append(out, b)
		}
	}
	return out
}
