package model

	batch = append(batch, logTxs, txs, queues)
	for _, d := range batch {
		d.BatchFindByHash(nil, hashDeles)
	}
}
