/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Command bench measures the throughput and the latency of a network (perf plan §4.2):
//
//	bench setup -node URL[,URL...] -key FILE [-algo ALGO] [-accounts N] [-file FILE]
//	bench run   -node URL[,URL...] -key FILE [-file FILE] [-workloads self,utxo,contract,write]
//	            [-rates 100,200,...] [-duration 30s] [-p99 2s] [-metrics URL[,URL...]] [-out DIR]
//
// setup makes the accounts of the bench, of an account algorithm, each a member of ecosystem 1
// with coins on its account and in UTXO, and the contract of the write-heavy workload, signed by
// the founder key (of the node keys' algorithm unless -key-algo tells another); it saves the keys
// of the accounts in the file. run takes the accounts of the file and, for each workload, sends
// transactions at each rate in turn for the duration, open loop and pre-signed, to the nodes in
// turn, while the network keeps up: every transaction in a block of every node and at least 80% of
// the rate in TPS. A step passes when, besides, the P99 of the latency to a block of every node is
// within the target. The latency is from the send to the time the bench saw the transaction in a
// block of the node, which it reads every -poll. It writes the steps, with the metrics of the nodes
// of -metrics (the /metrics of --metricsListen), as JSON and Markdown in -out, and exits 1 if no
// rate of a workload passed. setup on a file that exists makes only the contract, if it is missing.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

func main() {
	log.SetFlags(log.Ltime)
	if len(os.Args) < 2 || (os.Args[1] != "setup" && os.Args[1] != "run") {
		fmt.Fprintln(os.Stderr, "usage: bench setup|run [flags]; bench setup -h tells the flags")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	nodes := fs.String("node", "http://127.0.0.1:7079", "API hosts of nodes of the network, comma-separated")
	keyFile := fs.String("key", "", "file of the founder's private key (hex)")
	keyAlgo := fs.String("key-algo", "", "account algorithm of the founder key (default the node keys' algorithm)")
	file := fs.String("file", "bench-accounts.json", "file of the keys of the accounts")
	batch := fs.Int("batch", 100, "transactions a request")

	algo := fs.String("algo", "ECC_Secp256k1", "account algorithm of the accounts setup makes: ECC_P256, ECC_Secp256k1, SM2, MLDSA65 or MLDSA87")
	accounts := fs.Int("accounts", 1000, "accounts setup makes")
	fund := fs.String("fund", "", "coins of ecosystem 1 setup sends each account, in its smallest unit (default 90% of the founder's account split evenly); half go to its UTXO")
	timeout := fs.Duration("timeout", 5*time.Minute, "time a step of setup waits for its transactions")

	names := fs.String("workloads", "self,utxo,contract,write", "workloads run measures")
	rates := fs.String("rates", "100,200,500,1000,2000,3000,5000", "rates (transactions a second) run steps through")
	duration := fs.Duration("duration", 30*time.Second, "time a step sends")
	p99 := fs.Duration("p99", 2*time.Second, "P99 target of the latency to a block of every node")
	drain := fs.Duration("drain", 30*time.Second, "time a step waits for a transaction to reach a block after the last did")
	poll := fs.Duration("poll", 50*time.Millisecond, "time between two reads of the last block of a node")
	metrics := fs.String("metrics", "", "URLs of the /metrics of the nodes, comma-separated")
	out := fs.String("out", "bench-results", "directory of the reports")
	label := fs.String("label", "", "text the report starts with: the build, the machine, the network")
	_ = fs.Parse(os.Args[2:])

	if *keyFile == "" {
		fmt.Fprintln(os.Stderr, "bench: -key is required")
		os.Exit(2)
	}
	c, err := connect(splitList(*nodes), *keyFile, *keyAlgo)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("network %d of %d nodes: node keys %s, hash %s", c.info.NetworkID, len(c.nodes), c.info.Cryptoer, c.info.Hasher)

	if os.Args[1] == "setup" {
		_, statErr := os.Stat(*file)
		split := *fund == "" && statErr != nil
		if split {
			balance, _, err := c.nodes[0].balance(c.founder.address())
			if err != nil {
				log.Fatal(err)
			}
			*fund = balance
		}
		if *fund == "" {
			*fund = "0"
		}
		amount, err := decimal.NewFromString(*fund)
		if err != nil {
			log.Fatalf("-fund: %v", err)
		}
		if split {
			amount = amount.Mul(decimal.NewFromFloat(0.9)).Div(decimal.NewFromInt(int64(*accounts))).Floor()
		}
		if split {
			log.Printf("each account gets %s", amount)
		}
		if err := setup(c, setupConfig{file: *file, algo: *algo, accounts: *accounts, fund: amount, batch: *batch, timeout: *timeout}); err != nil {
			log.Fatal(err)
		}
		return
	}

	accs, err := loadAccounts(*file)
	if err != nil {
		log.Fatal(err)
	}
	cfg := runConfig{duration: *duration, batch: *batch, drain: *drain, p99: *p99, metrics: splitList(*metrics), poll: *poll}
	for _, r := range splitList(*rates) {
		v, err := strconv.Atoi(r)
		if err != nil || v < 1 {
			log.Fatalf("-rates: %q", r)
		}
		cfg.rates = append(cfg.rates, v)
	}
	// The run number keeps the transactions of this run unlike those of the runs before
	run := int(time.Now().Unix() % 1000)
	ws, err := workloads(c, accs, splitList(*names), run)
	if err != nil {
		log.Fatal(err)
	}
	r := &report{
		Time: time.Now(), Label: *label, Nodes: splitList(*nodes), NetworkID: c.info.NetworkID, Cryptoer: c.info.Cryptoer,
		Hasher: c.info.Hasher, Algo: accs[0].algo.String(), Accounts: len(accs), P99Ms: p99.Milliseconds(),
	}
	seq := 0
	for _, w := range ws {
		r.Steps = append(r.Steps, runWorkload(c, w, cfg, &seq)...)
	}
	r.finish()
	base, err := r.write(*out)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("report %s.md and .json", base)
	for _, w := range ws {
		if r.Max[w.name] == 0 {
			os.Exit(1)
		}
	}
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
