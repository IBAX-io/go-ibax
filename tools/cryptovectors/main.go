/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Command cryptovectors keeps the golden vectors that pin clients (Weaver) to the node's crypto
// and transaction decoding. Every vector file holds inputs plus what the node computes from them;
// the node side is recomputed here with go-ibax's own code.
//
//	go run ./tools/cryptovectors gen   [-dir tools/cryptovectors/testdata]
//	go run ./tools/cryptovectors check [-dir tools/cryptovectors/testdata]
//
// gen rewrites the node-side fields; check recomputes them and fails on any difference. ECDSA and
// SM2 signatures are randomized, so node signatures are verified instead of compared: gen keeps a
// stored signature while it still verifies.
package main

import (
	"flag"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
)

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "gen" && os.Args[1] != "check") {
		fmt.Fprintln(os.Stderr, "usage: cryptovectors gen|check [-dir path]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	dir := fs.String("dir", "tools/cryptovectors/testdata", "directory of the vector files")
	_ = fs.Parse(os.Args[2:])

	// Rejections are expected outcomes here, not node errors worth logging
	log.SetLevel(log.PanicLevel)

	diffs, err := run(*dir, os.Args[1] == "gen")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, d := range diffs {
		fmt.Fprintln(os.Stderr, d)
	}
	if len(diffs) > 0 {
		os.Exit(1)
	}
}
