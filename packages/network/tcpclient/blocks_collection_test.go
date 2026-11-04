/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package tcpclient

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/network"
)

// answer is the bytes of an answer of blocks of the data size with the bodies
func answer(dataSize int64, bodies ...[]byte) []byte {
	var b bytes.Buffer
	network.WriteInt(dataSize, &b)
	for _, body := range bodies {
		(&network.GetBodyResponse{Data: body}).Write(&b)
	}
	return b.Bytes()
}

func readBodies(ctx context.Context, data []byte, count int64) ([][]byte, error) {
	blocks, errs := GetBlockBodiesChan(ctx, BufCloser{bytes.NewBuffer(data)}, count)
	// the errors are read meanwhile, as GetBlocksBatch does
	done := make(chan error)
	go func() {
		var err error
		for e := range errs {
			if e != nil {
				err = e
			}
		}
		done <- err
	}()
	var got [][]byte
	for b := range blocks {
		got = append(got, append([]byte(nil), b...))
	}
	return got, <-done
}

// A node reads the blocks of an answer, and takes no length out of the bytes the answer said
func TestGetBlockBodiesChanLengths(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got, err := readBodies(ctx, answer(5, []byte("ab"), []byte("cde")), 2)
	if err != nil || len(got) != 2 || string(got[0]) != "ab" || string(got[1]) != "cde" {
		t.Fatalf("two blocks: %q, %v", got, err)
	}
	// a block longer than the bytes left
	if got, err = readBodies(ctx, answer(4, []byte("ab"), []byte("cde")), 2); !errors.Is(err, ErrorWrongSizeBytes) || len(got) != 1 {
		t.Fatalf("a block past the data size: %q, %v", got, err)
	}
	// a data size out of bounds
	for _, size := range []int64{-1, network.MaxBlocksBytes + 1} {
		if _, err = readBodies(ctx, answer(size, []byte("ab")), 1); !errors.Is(err, ErrorWrongSizeBytes) {
			t.Fatalf("data size %d: %v", size, err)
		}
	}
}

// A context done stops the reading of the blocks at once
func TestGetBlockBodiesChanCancel(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		server.Write(answer(10, []byte("ab")))
	}()
	blocks, errs := GetBlockBodiesChan(ctx, client, 2)
	go func() {
		for range errs {
		}
	}()
	if b := <-blocks; string(b) != "ab" {
		t.Fatalf("first block %q", b)
	}
	cancel()
	select {
	case _, open := <-blocks:
		if open {
			t.Fatal("a block after the cancel")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the reading goes on after the cancel")
	}
}
