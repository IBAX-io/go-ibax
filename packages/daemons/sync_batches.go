/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/network/bftnet"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"
)

// batch is the blocks a host sends for a request of the blocks from a block on
type batch struct {
	from   int64
	count  int64
	blocks <-chan []byte
	// certs are the certificates of the blocks of a certified batch, one for each of its count
	certs  [][]byte
	cancel context.CancelFunc
	err    error
}

// The requests of a batch, that tests replace
var (
	fetchBlocks       = tcpclient.GetBlocksBatch
	fetchCertificates = bftnet.FetchCertificates
)

// fetchBatch asks host for the blocks from the block from on, and for their certificates if
// certified, and sends the batch to the channel it returns once the host has said how many blocks
// it sends; the blocks go on coming while the batch waits to be played
func fetchBatch(ctx context.Context, host string, from int64, certified bool) <-chan batch {
	ch := make(chan batch, 1)
	go func() {
		ctx, cancel := context.WithCancel(ctx)
		b := batch{from: from, cancel: cancel}
		b.blocks, b.count, b.err = fetchBlocks(ctx, host, from, false)
		if b.err == nil && certified {
			b.certs, b.err = fetchBatchCertificates(host, from, b.count)
			// the blocks without a certificate go to the next batch
			b.count = min(b.count, int64(len(b.certs)))
		}
		if b.err != nil {
			cancel()
		}
		ch <- b
	}()
	return ch
}

// fetchBatchCertificates fetches the certificates of count heights from the height from, by
// requests of bftnet.MaxCertificates of them
func fetchBatchCertificates(host string, from, count int64) ([][]byte, error) {
	certs := make([][]byte, 0, count)
	for int64(len(certs)) < count {
		ask := min(count-int64(len(certs)), bftnet.MaxCertificates)
		got, err := fetchCertificates(host, uint64(from)+uint64(len(certs)), int(ask), bftFetchTimeout)
		if err != nil {
			return nil, err
		}
		certs = append(certs, got...)
		if int64(len(got)) < ask {
			break
		}
	}
	if len(certs) == 0 {
		// the host has blocks after it: the height is out of its window
		return nil, fmt.Errorf("%w: %s has no certificate of height %d", tcpclient.ErrNotKept, host, from)
	}
	return certs, nil
}

// playBatches plays the blocks of host from the block from on, by batches, till it has played the
// block to, and returns how many blocks it played. It asks for a batch as soon as the host has said
// how many blocks the batch before has, so a batch comes while the one before plays. play plays the
// blocks of a batch and returns how many it played: a batch it does not play whole stops it.
func playBatches(ctx context.Context, host string, from, to int64, certified bool, play func(b batch) (int64, error)) (int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	// a batch asked for but not played is dropped
	defer cancel()
	var played int64
	for next := fetchBatch(ctx, host, from, certified); next != nil; {
		b := <-next
		if b.err != nil {
			return played, b.err
		}
		next = nil
		if b.from+b.count <= to {
			next = fetchBatch(ctx, host, b.from+b.count, certified)
		}
		n, err := play(b)
		b.cancel()
		played += n
		if err != nil {
			return played, err
		}
		if n != b.count {
			return played, fmt.Errorf("%s sent %d of the %d blocks from %d", host, n, b.count, b.from)
		}
	}
	return played, nil
}
