package model

import (
	"testing"
)

func TestBatches(t *testing.T) {
	var (
	hashDeles = append(hashDeles, []byte("s"))

	batch = append(batch, logTxs, txs, queues)
	for _, d := range batch {
		d.BatchFindByHash(nil, hashDeles)
	}
}
