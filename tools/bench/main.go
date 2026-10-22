/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Command bench measures the throughput and the latency of a network (perf plan §4.2):
//
//	bench setup -node URL[,URL...] -key FILE [-algo ALGO] [-accounts N] [-file FILE]
//	bench run   -node URL[,URL...] -key FILE [-file FILE] [-workloads self,utxo,contract,write]
//	            [-rates 100,200,...] [-duration 30s] [-p99 2s] [-metrics URL[,URL...]] [-out DIR]
//	bench send  -node URL[,URL...] -key FILE -n N [-rate R] CONTRACT [NAME=VALUE...]
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
//
// send sends N transactions of a contract of ecosystem 1 signed by the founder, R a second (100 by
// default), open loop, and waits for them to reach blocks; {i} in a VALUE is the number of the
// transaction, 1 to N, so that no two are alike. It prints "sent N in_blocks K refused R seconds S"
// and "latency_ms p50 A p95 B max C" of the transactions in blocks, and exits 1 when a transaction
// reached no block.
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
	if len(os.Args) < 2 || (os.Args[1] != "setup" && os.Args[1] != "run" && os.Args[1] != "send") {
		fmt.Fprintln(os.Stderr, "usage: bench setup|run|send [flags]; bench setup -h tells the flags")
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

	names := fs.String("workloads", "self,utxo,transfer,contract,write", "workloads run measures: self (TransferSelf), utxo (UTXO), transfer (@1TokensSend), contract (a mix of 4 @1ProfileEdit, 3 @1NotificationsSend and 3 @1TokensSend), write (5 DBInsert and 5 DBUpdate a transaction)")
	rates := fs.String("rates", "100,200,500,1000,2000,3000,5000", "rates (transactions a second) run steps through")
	duration := fs.Duration("duration", 30*time.Second, "time a step sends")
	p99 := fs.Duration("p99", 2*time.Second, "P99 target of the latency to a block of every node")
	drain := fs.Duration("drain", 30*time.Second, "time a step waits for a transaction to reach a block after the last did")
	poll := fs.Duration("poll", 50*time.Millisecond, "time between two reads of the last block of a node")
	metrics := fs.String("metrics", "", "URLs of the /metrics of the nodes, comma-separated")
	out := fs.String("out", "bench-results", "directory of the reports")
	label := fs.String("label", "", "text the report starts with: the build, the machine, the network")

	count := fs.Int("n", 0, "transactions send sends")
	rate := fs.Int("rate", 100, "transactions a second send sends")
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

	if os.Args[1] == "send" {
		if *count < 1 || *rate < 1 || fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: bench send -node URL[,URL...] -key FILE -n N [-rate R] CONTRACT [NAME=VALUE...]")
			os.Exit(2)
		}
		os.Exit(send(c, fs.Arg(0), fs.Args()[1:], *count, *rate, runConfig{batch: *batch, drain: *drain, poll: *poll, p99: *p99}))
	}
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

// send runs the transactions of send and prints what came of them, as tools/cmd/load of dapps did;
// it tells the exit status
func send(c *chainClient, contract string, args []string, n, rate int, cfg runConfig) int {
	call, err := newContractCall(c.nodes[0], contract, args)
	if err != nil {
		log.Print(err)
		return 2
	}
	w := workload{name: contract, make: func(seq int) (*signedTx, error) {
		values, err := call.values(seq + 1)
		if err != nil {
			return nil, err
		}
		return c.signer.contract(c.founder, call.id, values)
	}}
	seq := 0
	s, err := runStep(c, w, rate, n, cfg, &seq)
	if err != nil {
		log.Print(err)
		return 1
	}
	fmt.Printf("sent %d in_blocks %d refused %d seconds %.1f\n", s.Sent, s.InBlock-s.FailedInBlock, s.Refused+s.FailedInBlock+s.Unsent, s.Seconds)
	if s.InBlock > 0 {
		fmt.Printf("latency_ms p50 %.0f p95 %.0f max %.0f\n", s.Latency.P50, s.Latency.P95, s.Latency.Max)
	}
	for why, k := range s.Failures {
		log.Printf("%d: %s", k, why)
	}
	if s.Lost > 0 {
		return 1
	}
	return 0
}
