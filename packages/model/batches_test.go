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

	hashDeles = append(hashDeles, []byte("s"))

	batch = append(batch, logTxs, txs, queues)
	for _, d := range batch {
