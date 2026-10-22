/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// report is the result of a run: the network, its accounts and the steps of each workload
type report struct {
	Time      time.Time `json:"time"`
	Label     string    `json:"label,omitempty"`
	Nodes     []string  `json:"nodes"`
	NetworkID int64     `json:"network_id"`
	Cryptoer  string    `json:"cryptoer"`
	Hasher    string    `json:"hasher"`
	Algo      string    `json:"algo"`
	Accounts  int       `json:"accounts"`
	P99Ms     int64     `json:"p99_target_ms"`
	Steps     []step    `json:"steps"`
	// Max is the highest rate of each workload at which a step passed
	Max map[string]int `json:"max_rate"`
}

func (r *report) finish() {
	r.Max = map[string]int{}
	for _, s := range r.Steps {
		if s.Pass && s.Rate > r.Max[s.Workload] {
			r.Max[s.Workload] = s.Rate
		}
	}
}

// write writes the report as bench-TIME.json and bench-TIME.md in the directory
func (r *report) write(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := filepath.Join(dir, "bench-"+r.Time.Format("20060102-150405"))
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(base+".json", b, 0o644); err != nil {
		return "", err
	}
	return base, os.WriteFile(base+".md", []byte(r.markdown()), 0o644)
}

func (r *report) markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Bench %s\n\n", r.Time.Format(time.RFC3339))
	if r.Label != "" {
		fmt.Fprintf(&b, "%s\n\n", r.Label)
	}
	fmt.Fprintf(&b, "| Item | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| Nodes | %d: %s |\n", len(r.Nodes), strings.Join(r.Nodes, ", "))
	fmt.Fprintf(&b, "| Network | %d, node keys %s, hash %s |\n", r.NetworkID, r.Cryptoer, r.Hasher)
	fmt.Fprintf(&b, "| Accounts | %d of %s |\n", r.Accounts, r.Algo)
	fmt.Fprintf(&b, "| P99 target | %d ms, to a block of every node |\n\n", r.P99Ms)

	b.WriteString("## Highest passing rate\n\n| Workload | Rate (tx/s) |\n|---|---|\n")
	names := []string{}
	for _, s := range r.Steps {
		if !slices.Contains(names, s.Workload) {
			names = append(names, s.Workload)
		}
	}
	for _, n := range names {
		fmt.Fprintf(&b, "| %s | %d |\n", n, r.Max[n])
	}

	b.WriteString("\n## Steps\n\n")
	b.WriteString("| Workload | Rate | Sent | In blocks (all nodes) | Failed | Refused | Lost | TPS | P50 / P95 / P99 / max ms, first node | P50 / P95 / P99 / max ms, all nodes | Blocks | Txs/block | Block interval ms | Pass |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	q := func(v quantiles) string { return fmt.Sprintf("%.0f / %.0f / %.0f / %.0f", v.P50, v.P95, v.P99, v.Max) }
	for _, s := range r.Steps {
		fmt.Fprintf(&b, "| %s | %d | %d | %d (%d) | %d | %d | %d | %.0f | %s | %s | %d | %.0f | %.0f | %v |\n",
			s.Workload, s.Rate, s.Sent, s.InBlock, s.InAll, s.FailedInBlock, s.Refused, s.Unsent+s.Lost, s.TPS,
			q(s.Latency), q(s.LatencyAll), s.Blocks, s.TxsPerBlock, s.BlockIntervalMs, s.Pass)
	}

	for _, s := range r.Steps {
		if len(s.Failures) == 0 && len(s.Nodes) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n### %s at %d/s\n\n", s.Workload, s.Rate)
		if len(s.Failures) > 0 {
			b.WriteString("| Failure | Count |\n|---|---|\n")
			for _, k := range sortedKeys(s.Failures) {
				fmt.Fprintf(&b, "| %s | %d |\n", strings.ReplaceAll(k, "|", "\\|"), s.Failures[k])
			}
			b.WriteString("\n")
		}
		if len(s.Nodes) > 0 {
			b.WriteString("| Node | Blocks by kind | Mean stage ms | Txs/block | Statements/tx | Tx stage µs | Signature checks | DBLock wait / hold s | CPU cores | RSS MiB |\n")
			b.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
			for _, p := range s.Nodes {
				fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %.0f | %.1f / %.1f | %.2f | %.0f |\n",
					p.Node, kv(p.Blocks, "%.0f"), kv(p.StageMs, "%.1f"), kv(p.TxsPerBlock, "%.0f"), kv(p.StatementsPerTx, "%.1f"),
					kv(p.TxStageUs, "%.0f"), p.SignVerifies, p.LockWaitS, p.LockHoldS, p.CPUCores, p.RSSMiB)
			}
		}
	}
	return b.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func kv(m map[string]float64, format string) string {
	parts := make([]string, 0, len(m))
	for _, k := range sortedKeys(m) {
		parts = append(parts, k+" "+fmt.Sprintf(format, m[k]))
	}
	return strings.Join(parts, "<br>")
}
