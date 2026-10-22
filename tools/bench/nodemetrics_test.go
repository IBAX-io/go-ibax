/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"strconv"
	"testing"
)

// TestProfileOfTheMetricsDelta checks that the profile of a step is the change of the metrics of a
// node over the step
func TestProfileOfTheMetricsDelta(t *testing.T) {
	scrape := func(blocks, seconds, txs, statements, verifies, cpu int) samples {
		text := `# TYPE ibax_block_stage_seconds histogram
ibax_block_stage_seconds_bucket{kind="generate",stage="total",le="+Inf"} ` + itoa(blocks) + `
ibax_block_stage_seconds_sum{kind="generate",stage="total"} ` + itoa(seconds) + `
ibax_block_stage_seconds_count{kind="generate",stage="total"} ` + itoa(blocks) + `
# TYPE ibax_block_txs histogram
ibax_block_txs_bucket{kind="generate",le="+Inf"} ` + itoa(blocks) + `
ibax_block_txs_sum{kind="generate"} ` + itoa(txs) + `
ibax_block_txs_count{kind="generate"} ` + itoa(blocks) + `
# TYPE ibax_block_db_statements histogram
ibax_block_db_statements_bucket{kind="generate",le="+Inf"} ` + itoa(blocks) + `
ibax_block_db_statements_sum{kind="generate"} ` + itoa(statements) + `
ibax_block_db_statements_count{kind="generate"} ` + itoa(blocks) + `
# TYPE ibax_sign_verify_seconds histogram
ibax_sign_verify_seconds_bucket{algo="ECC_Secp256k1",le="+Inf"} ` + itoa(verifies) + `
ibax_sign_verify_seconds_sum{algo="ECC_Secp256k1"} 1
ibax_sign_verify_seconds_count{algo="ECC_Secp256k1"} ` + itoa(verifies) + `
# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total ` + itoa(cpu) + `
`
		s, err := parseMetrics(text)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	p := profileOf("n", delta(scrape(2, 1, 100, 400, 300, 10), scrape(6, 9, 900, 4400, 2700, 30)), 10)
	for name, got := range map[string]float64{
		"generate blocks": p.Blocks["generate"], "total ms": p.StageMs["generate/total"], "txs a block": p.TxsPerBlock["generate"],
		"statements a tx": p.StatementsPerTx["generate"], "signature checks": p.SignVerifies, "cores": p.CPUCores,
	} {
		want := map[string]float64{"generate blocks": 4, "total ms": 2000, "txs a block": 200, "statements a tx": 5, "signature checks": 2400, "cores": 2}[name]
		if got != want {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
