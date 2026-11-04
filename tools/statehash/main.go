/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Command statehash computes the state hash of the database of a node as the node does at a
// snapshot height (packages/snapshot), and measures it: the time it takes, the rows and the bytes
// of each table, and with -out the files of the snapshot.
//
//	go run ./tools/statehash -dsn 'host=... dbname=...' [-workers 4] [-hasher SHA256] [-out dir]
package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/snapshot"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := flag.String("dsn", "", "the database of the node")
	workers := flag.Int("workers", 4, "the workers that encode the tables")
	hasher := flag.String("hasher", "SHA256", "the hash function of the chain")
	out := flag.String("out", "", "a directory to write the encoding of each table to")
	flag.Parse()
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "usage: statehash -dsn DSN [-workers N] [-hasher NAME] [-out DIR]")
		os.Exit(2)
	}
	if err := run(*dsn, *workers, *hasher, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dsn string, workers int, hasher, out string) error {
	crypto.InitHashAlgo(hasher)
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return err
	}
	db := stdlib.OpenDB(*cfg)
	defer db.Close()
	var sink snapshot.Sink
	if out != "" {
		if err := os.MkdirAll(out, 0o755); err != nil {
			return err
		}
		sink = func(table string) (io.WriteCloser, error) {
			return os.Create(filepath.Join(out, table+".copy"))
		}
	}
	ctx := context.Background()
	start := time.Now()
	s, err := snapshot.Begin(ctx, db)
	if err != nil {
		return err
	}
	defer s.Close()
	begun := time.Since(start)
	state, err := s.Hash(ctx, workers, sink)
	if err != nil {
		return err
	}
	took := time.Since(start)
	tables := slices.Clone(state.Tables)
	slices.SortFunc(tables, func(a, b snapshot.Table) int { return cmp.Compare(b.Bytes, a.Bytes) })
	var rows, bytes int64
	for _, t := range tables {
		rows += t.Rows
		bytes += t.Bytes
		fmt.Printf("table %-32s rows %12d bytes %14d chunks %6d\n", t.Name, t.Rows, t.Bytes, len(t.Chunks))
	}
	fmt.Printf("state hash %x: %d tables, %d rows, %d bytes, begin %v, total %v with %d workers\n",
		state.Hash, len(tables), rows, bytes, begun.Round(time.Millisecond), took.Round(time.Millisecond), workers)
	return nil
}
