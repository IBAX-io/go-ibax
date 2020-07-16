package model

import (
	"testing"
)

func TestBatches(t *testing.T) {
	var (
		logTxs = new(logTxser)
		txs    = new(txser)
