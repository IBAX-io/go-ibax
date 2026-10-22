/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"fmt"
	"log"
	"runtime"
	"slices"
	"sync"
	"time"
)

// runConfig is how a run steps up the rate of each workload
type runConfig struct {
	rates    []int
	duration time.Duration
	batch    int
	drain    time.Duration
	p99      time.Duration
	metrics  []string
	poll     time.Duration
}

// step is the result of a workload at a rate
type step struct {
	Workload string  `json:"workload"`
	Rate     int     `json:"rate"`
	Seconds  float64 `json:"seconds"`
	Sent     int     `json:"sent"`
	// Unsent are those the node refused to take in the request that sent them
	Unsent int `json:"unsent"`
	// InBlock are in a block of a node, InAll in a block of every node
	InBlock int `json:"in_block"`
	InAll   int `json:"in_all"`
	// FailedInBlock are in a block and failed in it
	FailedInBlock int `json:"failed_in_block"`
	// Refused are those a node refused after it took them; Lost are in no block and not refused
	Refused int `json:"refused"`
	Lost    int `json:"lost"`
	// TPS is the transactions in blocks over the seconds from the first sent to the last in a block
	TPS float64 `json:"tps"`
	// Latency is from the send to the block of the first node, LatencyAll to that of the last
	Latency    quantiles `json:"latency_ms"`
	LatencyAll quantiles `json:"latency_all_ms"`
	// Blocks are the blocks of the first node in the step, of TxsPerBlock transactions on average,
	// BlockIntervalMs apart on average
	Blocks          int            `json:"blocks"`
	TxsPerBlock     float64        `json:"txs_per_block"`
	BlockIntervalMs float64        `json:"block_interval_ms"`
	Failures        map[string]int `json:"failures,omitempty"`
	Nodes           []nodeProfile  `json:"nodes,omitempty"`
	// Pass is whether every transaction reached a block of every node, at the rate, within the P99
	Pass bool `json:"pass"`
}

type quantiles struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
	Max float64 `json:"max"`
}

func quantilesOf(d []time.Duration) quantiles {
	if len(d) == 0 {
		return quantiles{}
	}
	slices.Sort(d)
	at := func(q float64) float64 { return float64(d[int(float64(len(d)-1)*q)].Microseconds()) / 1000 }
	return quantiles{P50: at(0.50), P95: at(0.95), P99: at(0.99), Max: at(1)}
}

// runWorkload steps up the rate of a workload while the network keeps up with it: every
// transaction in a block and at least 80% of the rate in TPS, whether the P99 passes or not; it
// tells each step
func runWorkload(c *chainClient, w workload, cfg runConfig, seq *int) []step {
	var out []step
	for _, rate := range cfg.rates {
		s, err := runStep(c, w, rate, rate*int(cfg.duration/time.Second), cfg, seq)
		if err != nil {
			log.Printf("%s at %d/s: %v", w.name, rate, err)
			break
		}
		out = append(out, *s)
		log.Printf("%s at %d/s: sent %d, in blocks %d (all nodes %d), failed %d, refused %d, lost %d, %.0f TPS, P99 %.0f ms (all nodes %.0f ms), pass %v",
			w.name, rate, s.Sent, s.InBlock, s.InAll, s.FailedInBlock, s.Refused, s.Lost, s.TPS, s.Latency.P99, s.LatencyAll.P99, s.Pass)
		if s.Unsent+s.Refused+s.Lost > 0 || s.TPS < 0.8*float64(rate) {
			break
		}
	}
	return out
}

// runStep sends total transactions, rate a second, open loop, pre-signed, to the nodes in turn, and
// waits for them to reach blocks
func runStep(c *chainClient, w workload, rate, total int, cfg runConfig, seq *int) (*step, error) {
	txs, err := presign(w, *seq, total)
	if err != nil {
		return nil, err
	}
	*seq += total

	watch, err := startWatcher(c.nodes, cfg.poll)
	if err != nil {
		return nil, err
	}
	before := scrapeAll(cfg.metrics)

	sentAt := make([]time.Time, total)
	unsent := make([]bool, total)
	var unsentMu sync.Mutex
	failures := map[string]int{}
	type batch struct{ from, to int }
	queues := make([]chan batch, len(c.nodes))
	var senders sync.WaitGroup
	for i, n := range c.nodes {
		// Several requests at a time to each node, so that a slow answer delays no send
		queues[i] = make(chan batch, 1024)
		for range 8 {
			senders.Add(1)
			go func() {
				defer senders.Done()
				for b := range queues[i] {
					if err := n.send(txs[b.from:b.to]); err != nil {
						unsentMu.Lock()
						for j := b.from; j < b.to; j++ {
							unsent[j] = true
						}
						failures["send: "+trim(err.Error())] += b.to - b.from
						unsentMu.Unlock()
					}
				}
			}()
		}
	}
	start := time.Now()
	tick := time.NewTicker(10 * time.Millisecond)
	next, turn := 0, 0
	for next < total {
		<-tick.C
		due := min(total, int(float64(rate)*time.Since(start).Seconds()))
		for next < due {
			to := min(due, next+cfg.batch)
			now := time.Now()
			for j := next; j < to; j++ {
				sentAt[j] = now
			}
			queues[turn%len(queues)] <- batch{next, to}
			turn++
			next = to
		}
	}
	tick.Stop()
	for _, q := range queues {
		close(q)
	}
	senders.Wait()
	sendEnd := time.Now()

	hashes := make([]string, 0, total)
	for j, tx := range txs {
		if !unsent[j] {
			hashes = append(hashes, tx.hash)
		}
	}
	// Waits until every transaction is in a block of every node, or none came for the drain time
	lastAll, lastChange := 0, time.Now()
	for {
		_, all := watch.count(hashes)
		if all == len(hashes) {
			break
		}
		if all != lastAll {
			lastAll, lastChange = all, time.Now()
		} else if time.Since(lastChange) > cfg.drain {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	watch.close()
	seconds := time.Since(start).Seconds()
	after := scrapeAll(cfg.metrics)

	s := &step{Workload: w.name, Rate: rate, Sent: total, Failures: failures}
	var lat, latAll []time.Duration
	var last time.Time
	var missing []string
	for j, tx := range txs {
		if unsent[j] {
			s.Unsent++
			continue
		}
		first, all, inFirst, inAll := watch.inBlock(tx.hash)
		if !inFirst {
			missing = append(missing, tx.hash)
			continue
		}
		s.InBlock++
		lat = append(lat, first.Sub(sentAt[j]))
		if inAll {
			s.InAll++
			latAll = append(latAll, all.Sub(sentAt[j]))
			if all.After(last) {
				last = all
			}
		}
	}
	s.Latency, s.LatencyAll = quantilesOf(lat), quantilesOf(latAll)
	if !last.IsZero() {
		s.Seconds = last.Sub(start).Seconds()
		s.TPS = float64(s.InAll) / s.Seconds
	}

	// The status of each tells those that failed in their block, and why those in none are not
	statuses := map[string]*txStatus{}
	for i := 0; i < len(hashes); i += 1000 {
		part, err := c.nodes[0].statuses(hashes[i:min(i+1000, len(hashes))])
		if err != nil {
			return nil, err
		}
		for h, st := range part {
			statuses[h] = st
		}
	}
	for _, h := range hashes {
		st := statuses[h]
		if st == nil || st.failure() == "" {
			continue
		}
		failures[trim(st.failure())]++
		if _, _, inFirst, _ := watch.inBlock(h); inFirst {
			s.FailedInBlock++
		}
	}
	for _, h := range missing {
		if st := statuses[h]; st != nil && st.failure() != "" {
			s.Refused++
		} else {
			s.Lost++
		}
	}

	blocks := watch.blocksBetween(start, start.Add(time.Duration(seconds*float64(time.Second))))
	for i := len(blocks) - 1; i >= 0 && blocks[i].txs == 0; i-- {
		blocks = blocks[:i]
	}
	for len(blocks) > 0 && blocks[0].txs == 0 {
		blocks = blocks[1:]
	}
	if len(blocks) > 0 {
		s.Blocks = len(blocks)
		n := 0
		for _, b := range blocks {
			n += b.txs
		}
		s.TxsPerBlock = float64(n) / float64(len(blocks))
		if len(blocks) > 1 {
			s.BlockIntervalMs = float64(blocks[len(blocks)-1].at.Sub(blocks[0].at).Milliseconds()) / float64(len(blocks)-1)
		}
	}
	for i, url := range cfg.metrics {
		if before[i] == nil || after[i] == nil {
			continue
		}
		s.Nodes = append(s.Nodes, profileOf(url, delta(before[i], after[i]), seconds))
	}
	s.Pass = s.Unsent == 0 && s.Refused == 0 && s.Lost == 0 && s.FailedInBlock == 0 && s.InAll == s.Sent &&
		s.LatencyAll.P99 <= float64(cfg.p99.Milliseconds()) &&
		sendEnd.Sub(start) <= time.Duration(total/rate+1)*time.Second
	return s, nil
}

// presign signs the transactions of a step beforehand, on every core
func presign(w workload, from, total int) ([]*signedTx, error) {
	txs := make([]*signedTx, total)
	errs := make([]error, runtime.NumCPU())
	var wg sync.WaitGroup
	for k := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := k; j < total; j += len(errs) {
				tx, err := w.make(from + j)
				if err != nil {
					errs[k] = err
					return
				}
				txs[j] = tx
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("signing: %w", err)
		}
	}
	return txs, nil
}

func scrapeAll(urls []string) []samples {
	out := make([]samples, len(urls))
	for i, u := range urls {
		text, err := scrape(u)
		if err != nil {
			log.Printf("metrics %s: %v", u, err)
			continue
		}
		if out[i], err = parseMetrics(text); err != nil {
			log.Printf("metrics %s: %v", u, err)
		}
	}
	return out
}

func trim(s string) string {
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}
