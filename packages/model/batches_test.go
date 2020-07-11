package model

import (
	"testing"
)

func TestBatches(t *testing.T) {
	var (
		logTxs = new(logTxser)
		txs    = new(txser)
		queues = new(queueser) //old not check that
		batch  []Batcher
	)
	var hashDeles ArrHashes
