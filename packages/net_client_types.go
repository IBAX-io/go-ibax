/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
	HostWithMaxBlock(hosts []string) (host string, maxBlockID int64, err error)
	GetBlocksBodies(host string, startBlock int64, blocksCount int, reverseOrder bool) (chan []byte, error)
	SendTransactions(host string, txes []model.Transaction) error
}
