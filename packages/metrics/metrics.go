/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package metrics holds the Prometheus metrics of the node and serves them, with the Go profiler,
// on a listen address of its own. The metrics measure where the time of a block goes: the stages of
// its execution, the transactions, the statements it sends to the database, the signatures it
// checks and the wait for the lock of the daemons.
package metrics

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"runtime"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

const namespace = "ibax"

// The kinds of a block execution
const (
	KindGenerate = "generate" // the node makes the block
	KindTrial    = "trial"    // a BFT trial, held for the decision of its height
	KindHeld     = "held"     // the node commits the trial it held of the block its height decided
	KindApply    = "apply"    // the node applies a block another node made
	KindReceived = "received" // a block the node got from another node, parsed before it runs
	KindPropose  = "propose"  // a proposer of the BFT consensus makes its proposal
)

// The stages of a block execution
const (
	StageBegin    = "begin"    // taking the snapshot of the memory and the database transaction of a trial
	StageExpand   = "expand"   // filling in the block of a compact proposal from the queue and the proposer
	StageVerify   = "verify"   // verifying in parallel the signatures of the transactions of another node's block
	StagePrepare  = "prepare"  // reading the outputs and the parameters the transactions need
	StageExecute  = "execute"  // running the transactions
	StageAfter    = "after"    // writing the logs, the statuses and the rollbacks of the transactions
	StageStream   = "stream"   // writing the logs, the statuses and the rollbacks of the committed transactions while the block runs
	StageInsert   = "insert"   // inserting the block
	StageCommit   = "commit"   // committing the database transaction
	StageNotify   = "notify"   // sending the notifications
	StageTotal    = "total"    // the whole execution
	StageParse    = "parse"    // parsing the block from its bytes
	StageRollback = "rollback" // rolling back a trial that failed, or one held of a block not decided
	StageHold     = "hold"     // holding a trial: the snapshot of the memory it changed, and the memory set back
	StageWait     = "wait"     // the proposer waits for its clock to pass the time of the last block
	StageCollect  = "collect"  // the proposer takes the transactions of its block from the queue
	StageCheck    = "check"    // reading the previous header, checking the header, the transactions and the sign of a block
	StageTrial    = "trial"    // the proposer runs its block
	StageBehind   = "behind"   // from the time of the proposed block to the end of the proposal
	StageValid    = "valid"    // valid(v) of a proposal from its arrival, by the speculation or by Validate
	StageLead     = "lead"     // from the end of the node's last block, the pool not empty, to the start of the collection of its next one
	StageCycle    = "cycle"    // from the end of the node's last block, the pool not empty, to the end of its next one
)

// The stages of a transaction execution
const (
	TxSavepoint = "savepoint"
	TxPlay      = "play"
	TxRollback  = "rollback_savepoint"
)

// durationBuckets cover 25 µs to about 52 s
var durationBuckets = prometheus.ExponentialBuckets(0.000025, 2, 22)

// contractBuckets cover 25 µs to about 1.6 s: a run of a transaction past it is past the time of a
// block
var contractBuckets = prometheus.ExponentialBuckets(0.000025, 2, 17)

var (
	registry = prometheus.NewRegistry()

	// BlockStage is the time of each stage of a block execution
	BlockStage = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "block_stage_seconds",
		Help:    "Time of each stage of a block execution.",
		Buckets: durationBuckets,
	}, []string{"kind", "stage"})

	// BlockTxs is the number of transactions of an executed block
	BlockTxs = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "block_txs",
		Help:    "Transactions of an executed block.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 14),
	}, []string{"kind"})

	// BlockStatements is the number of database statements of a block execution
	BlockStatements = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "block_db_statements",
		Help:    "Database statements of a block execution.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 20),
	}, []string{"kind"})

	// TxStage is the time of each stage of a transaction execution
	TxStage = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "tx_stage_seconds",
		Help:    "Time of each stage of a transaction execution.",
		Buckets: durationBuckets,
	}, []string{"stage"})

	// TxResults counts the executed transactions by their result
	TxResults = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "tx_results_total",
		Help: "Executed transactions by result.",
	}, []string{"kind", "result"})

	// TxRuns counts the runs of the transactions of the blocks the state of the block commits, by
	// how they ran: in parallel; again after the transactions before them committed, as their run
	// in parallel read what those wrote; in order, as the state runs them only in order
	TxRuns = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "tx_runs_total",
		Help: "Committed runs of transactions by how they ran: parallel, early, again, ordered.",
	}, []string{"run"})

	// TxExecutions counts every run of a transaction of the blocks the state of the block runs,
	// committed or not, by why it ran: parallel, the first run of the transaction on a worker;
	// early, again on a worker after a commit woke it; recheck, again on a worker at once, as a
	// commit before it was a reader changed what it read; again, at its commit; ordered, in
	// order. The runs of a block that do not hold are those past its TxRuns.
	TxExecutions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "tx_executions_total",
		Help: "Runs of transactions, committed or not, by why they ran: parallel, early, recheck, again, ordered.",
	}, []string{"run"})

	// ContractTime is the time of each run of a transaction, by its contract and the part of the
	// time: state, the time of its statements on the state of the block and PostgreSQL, with the
	// reading of the rows they returned; other, the rest of the run, the virtual machine of the
	// contract the most of it
	ContractTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "contract_run_seconds",
		Help:    "Time of a run of a transaction by contract and part: state, other.",
		Buckets: contractBuckets,
	}, []string{"contract", "part"})

	// ContractFuel is the fuel of each run of a transaction, by its contract
	ContractFuel = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "contract_run_fuel",
		Help:    "Fuel of a run of a transaction by contract.",
		Buckets: prometheus.ExponentialBuckets(1, 4, 16),
	}, []string{"contract"})

	// ContractStatements is the number of database statements of each run of a transaction, by
	// its contract
	ContractStatements = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "contract_run_statements",
		Help:    "Database statements of a run of a transaction by contract.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12),
	}, []string{"contract"})

	// ContractRows counts the rows the runs of the transactions read and wrote, by contract
	ContractRows = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "contract_rows_total",
		Help: "Rows the runs of transactions read and wrote by contract and op: read, written.",
	}, []string{"contract", "op"})

	// ContractRuns is TxRuns by contract: how the committed runs of its transactions ran
	ContractRuns = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "contract_runs_total",
		Help: "Committed runs of transactions by contract and how they ran: parallel, early, again, ordered.",
	}, []string{"contract", "run"})

	// ContractResults counts the transactions of the blocks by contract and result: ok; penalty,
	// the contract failed and paid; failed, the transaction failed; time_long, the time left to
	// the block stopped a run that took more than 50 ms; time_short, it stopped a shorter run, as
	// the block had little time left; limit, another limit of the block stopped it
	ContractResults = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "contract_results_total",
		Help: "Transactions by contract and result: ok, penalty, failed, time_long, time_short, limit.",
	}, []string{"contract", "result"})

	// Speculations counts how the speculations of streaming proposals ended: finished, as the
	// proposal went on from them; refused, as they decided a proposal is not valid; validated, as
	// they did not decide the proposal and the node tried it from the start; aborted, with no
	// proposal of theirs
	Speculations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "speculations_total",
		Help: "Speculations of streaming proposals by how they ended: finished, refused, validated, aborted.",
	}, []string{"result"})

	// StateFallbacks counts what the state of the block ran in PostgreSQL or only in order, by
	// reason
	StateFallbacks = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "state_fallbacks_total",
		Help: "Statements the state of the block ran in PostgreSQL or only in order, by reason.",
	}, []string{"reason"})

	// StateFlush is the time the state of a block takes to write the rows it changed to
	// PostgreSQL at the end of the block
	StateFlush = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace, Name: "state_flush_seconds",
		Help:    "Time to write the rows the state of a block changed to PostgreSQL.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
	})

	// StateFlushRows counts the rows the state of the blocks wrote to PostgreSQL, by operation
	StateFlushRows = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "state_flush_rows_total",
		Help: "Rows the state of the blocks wrote to PostgreSQL by operation: insert, update, delete.",
	}, []string{"op"})

	// DBStatements counts the database statements by their operation
	DBStatements = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "db_statements_total",
		Help: "Database statements by operation.",
	}, []string{"op"})

	// DBCache counts the reads of a block transaction its cache answers (hit) or not (miss)
	DBCache = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "db_cache_total",
		Help: "Reads of the database a block transaction repeats, answered by its cache (hit) or not (miss).",
	}, []string{"result"})

	// SignVerify is the time of a signature check
	SignVerify = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "sign_verify_seconds",
		Help:    "Time of a signature check, by algorithm.",
		Buckets: durationBuckets,
	}, []string{"algo"})

	// SignVerifyCached is the signature checks a signature verified before answers
	SignVerifyCached = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "sign_verify_cached_total",
		Help: "Signature checks answered by a signature verified before, by algorithm.",
	}, []string{"algo"})

	// TxPool is the transactions of the pool of the node and the bytes of their data
	TxPool = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace, Name: "tx_pool",
		Help: "Transactions in the pool of the node (count) and the bytes of their data (bytes).",
	}, []string{"of"})

	// LockWait is the time a daemon waits for the lock of the daemons
	LockWait = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace, Name: "dblock_wait_seconds",
		Help:    "Time waiting for the lock of the daemons.",
		Buckets: durationBuckets,
	})

	// LockHold is the time a daemon holds the lock of the daemons
	LockHold = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace, Name: "dblock_hold_seconds",
		Help:    "Time holding the lock of the daemons.",
		Buckets: durationBuckets,
	})

	// DaemonRun is the time of a run of a daemon
	DaemonRun = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "daemon_run_seconds",
		Help:    "Time of a run of a daemon.",
		Buckets: durationBuckets,
	}, []string{"daemon"})

	// APIRequest is the time of a request of the API
	APIRequest = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "api_request_seconds",
		Help:    "Time of an API request, by route.",
		Buckets: durationBuckets,
	}, []string{"api", "method", "route"})

	// TCPRequest is the time of a request a node serves to another node
	TCPRequest = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "tcp_request_seconds",
		Help:    "Time of a node-to-node request, by type.",
		Buckets: durationBuckets,
	}, []string{"type"})
)

func init() {
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		BlockStage, BlockTxs, BlockStatements, TxStage, TxResults, TxRuns, TxExecutions,
		ContractTime, ContractFuel, ContractStatements, ContractRows, ContractRuns, ContractResults, Speculations, StateFallbacks, StateFlush, StateFlushRows, DBStatements, DBCache,
		SignVerify, SignVerifyCached, TxPool, LockWait, LockHold, DaemonRun, APIRequest, TCPRequest,
	)
}

// Since is the seconds from t
func Since(t time.Time) float64 {
	return time.Since(t).Seconds()
}

// Handler serves the metrics and the profiler
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("/debug/pprof/rates", rates)
	return mux
}

// rates sets the rates of the mutex and the block profiles, which are off unless set: mutex is the
// fraction 1/mutex of the contention events it takes, block the nanoseconds of blocking a sample
// stands for, and 0 turns either off
func rates(w http.ResponseWriter, r *http.Request) {
	set := map[string]func(int){
		"mutex": func(n int) { runtime.SetMutexProfileFraction(n) },
		"block": runtime.SetBlockProfileRate,
	}
	values := map[string]int{}
	for name := range set {
		v := r.URL.Query().Get(name)
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			http.Error(w, fmt.Sprintf("%s: %q is not a rate", name, v), http.StatusBadRequest)
			return
		}
		values[name] = n
	}
	for name, n := range values {
		set[name](n)
	}
	fmt.Fprintf(w, "mutex %d\n", runtime.SetMutexProfileFraction(-1))
}

// CheckListen checks the listen address of the metrics: empty turns them off, otherwise it is a
// host and a port, and the host is a loopback address, since the profiler shows the memory of the
// node and its requests take its time.
func CheckListen(listen string) error {
	if listen == "" {
		return nil
	}
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("metrics listen address %q: %w", listen, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("metrics listen address %q: the host is not a loopback address", listen)
	}
	return nil
}

// Serve serves the metrics and the profiler on listen until the server fails; empty listen serves
// nothing
func Serve(listen string) error {
	if listen == "" {
		return nil
	}
	if err := CheckListen(listen); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.WithError(err).Error("metrics server stopped")
		}
	}()
	return nil
}
