/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"sort"
	"strings"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// sample is the value of a metric series: a counter, or the count and the sum of a histogram
type sample struct {
	count, sum float64
}

// samples are the series of a scrape, by the name of the metric and its labels
type samples map[string]sample

// parseMetrics reads the series of the metrics of a node
func parseMetrics(text string) (samples, error) {
	p := expfmt.NewTextParser(model.UTF8Validation)
	families, err := p.TextToMetricFamilies(strings.NewReader(text))
	if err != nil {
		return nil, err
	}
	out := samples{}
	for name, f := range families {
		for _, m := range f.GetMetric() {
			key := seriesKey(name, m.GetLabel())
			switch {
			case m.Histogram != nil:
				out[key] = sample{count: float64(m.Histogram.GetSampleCount()), sum: m.Histogram.GetSampleSum()}
			case m.Counter != nil:
				out[key] = sample{count: m.Counter.GetValue(), sum: m.Counter.GetValue()}
			case m.Gauge != nil:
				out[key] = sample{count: m.Gauge.GetValue(), sum: m.Gauge.GetValue()}
			}
		}
	}
	return out, nil
}

func seriesKey(name string, labels []*dto.LabelPair) string {
	if len(labels) == 0 {
		return name
	}
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		parts = append(parts, l.GetName()+"="+l.GetValue())
	}
	sort.Strings(parts)
	return name + "{" + strings.Join(parts, ",") + "}"
}

// delta is the change of the series from before to after; a gauge keeps its value after
func delta(before, after samples) samples {
	out := samples{}
	for k, a := range after {
		b := before[k]
		if strings.HasPrefix(k, "process_resident_memory_bytes") || strings.HasPrefix(k, "go_goroutines") {
			out[k] = a
			continue
		}
		out[k] = sample{count: a.count - b.count, sum: a.sum - b.sum}
	}
	return out
}

// nodeProfile is what a node spent on the transactions of a step: the blocks it executed, the mean
// time of each stage of them, the statements and the signature checks
type nodeProfile struct {
	Node string `json:"node"`
	// Blocks are the block executions by kind (generate, trial, apply)
	Blocks map[string]float64 `json:"blocks"`
	// StageMs is the mean time of each stage by kind, "kind/stage"
	StageMs map[string]float64 `json:"stage_ms"`
	// TxsPerBlock is the mean of the transactions of an executed block, by kind
	TxsPerBlock map[string]float64 `json:"txs_per_block"`
	// StatementsPerTx is the database statements of block executions per transaction, by kind
	StatementsPerTx map[string]float64 `json:"statements_per_tx"`
	// TxStageUs is the mean time of each stage of a transaction
	TxStageUs map[string]float64 `json:"tx_stage_us"`
	// SignVerifies is the count of signature checks
	SignVerifies float64 `json:"sign_verifies"`
	// LockWaitS and LockHoldS are the time spent waiting for and holding DBLock
	LockWaitS float64 `json:"dblock_wait_s"`
	LockHoldS float64 `json:"dblock_hold_s"`
	// CPUCores is the mean CPU the node used: its CPU seconds over the seconds of the step
	CPUCores float64 `json:"cpu_cores"`
	// RSSMiB is the resident memory of the node at the end
	RSSMiB float64 `json:"rss_mib"`
	// Statements are all the statements of the node by operation
	Statements map[string]float64 `json:"statements"`
}

func profileOf(node string, d samples, seconds float64) nodeProfile {
	p := nodeProfile{
		Node: node, Blocks: map[string]float64{}, StageMs: map[string]float64{}, TxsPerBlock: map[string]float64{},
		StatementsPerTx: map[string]float64{}, TxStageUs: map[string]float64{}, Statements: map[string]float64{},
	}
	labels := func(key, prefix string) map[string]string {
		out := map[string]string{}
		inner := strings.TrimSuffix(strings.TrimPrefix(key, prefix+"{"), "}")
		for _, kv := range strings.Split(inner, ",") {
			k, v, _ := strings.Cut(kv, "=")
			out[k] = v
		}
		return out
	}
	txs := map[string]float64{}
	for key, s := range d {
		switch {
		case strings.HasPrefix(key, "ibax_block_stage_seconds{"):
			l := labels(key, "ibax_block_stage_seconds")
			if s.count > 0 {
				p.StageMs[l["kind"]+"/"+l["stage"]] = s.sum / s.count * 1000
			}
			if l["stage"] == "total" && s.count > 0 {
				p.Blocks[l["kind"]] = s.count
			}
		case strings.HasPrefix(key, "ibax_block_txs{"):
			l := labels(key, "ibax_block_txs")
			txs[l["kind"]] = s.sum
			if s.count > 0 {
				p.TxsPerBlock[l["kind"]] = s.sum / s.count
			}
		case strings.HasPrefix(key, "ibax_tx_stage_seconds{"):
			l := labels(key, "ibax_tx_stage_seconds")
			if s.count > 0 {
				p.TxStageUs[l["stage"]] = s.sum / s.count * 1e6
			}
		case strings.HasPrefix(key, "ibax_sign_verify_seconds{"):
			p.SignVerifies += s.count
		case strings.HasPrefix(key, "ibax_db_statements_total{"):
			p.Statements[labels(key, "ibax_db_statements_total")["op"]] = s.count
		case key == "ibax_dblock_wait_seconds":
			p.LockWaitS = s.sum
		case key == "ibax_dblock_hold_seconds":
			p.LockHoldS = s.sum
		case key == "process_cpu_seconds_total" && seconds > 0:
			p.CPUCores = s.count / seconds
		case key == "process_resident_memory_bytes":
			p.RSSMiB = s.count / (1 << 20)
		}
	}
	for key, s := range d {
		if strings.HasPrefix(key, "ibax_block_db_statements{") {
			kind := labels(key, "ibax_block_db_statements")["kind"]
			if txs[kind] > 0 {
				p.StatementsPerTx[kind] = s.sum / txs[kind]
			}
		}
	}
	return p
}
