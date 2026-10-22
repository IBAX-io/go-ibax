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
	KindTrial    = "trial"    // a BFT trial, rolled back
	KindApply    = "apply"    // the node applies a block another node made
	KindReceived = "received" // a block the node got from another node, parsed before it runs
)

// The stages of a block execution
const (
	StagePrepare  = "prepare"  // reading the outputs and the parameters the transactions need
	StageExecute  = "execute"  // running the transactions
	StageAfter    = "after"    // writing the logs, the statuses and the rollbacks of the transactions
	StageInsert   = "insert"   // inserting the block
	StageCommit   = "commit"   // committing the database transaction
	StageNotify   = "notify"   // sending the notifications
	StageTotal    = "total"    // the whole execution
	StageParse    = "parse"    // parsing the block from its bytes
	StageRollback = "rollback" // rolling back a trial
)

// The stages of a transaction execution
const (
	TxSavepoint = "savepoint"
	TxPlay      = "play"
	TxRollback  = "rollback_savepoint"
)

// durationBuckets cover 25 µs to about 52 s
var durationBuckets = prometheus.ExponentialBuckets(0.000025, 2, 22)

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
		BlockStage, BlockTxs, BlockStatements, TxStage, TxResults, DBStatements, DBCache,
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
	return mux
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
