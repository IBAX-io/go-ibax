/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package crypto

import (
	"strings"
	"testing"
)

func TestRandSeq(t *testing.T) {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		s := RandSeq(16)
		if len(s) != 16 || strings.Trim(s, letters) != "" {
			t.Fatalf("RandSeq(16) = %q", s)
		}
		if seen[s] {
			t.Fatalf("RandSeq repeated %q", s)
		}
		seen[s] = true
	}
	if s := RandSeq(0); s != "" {
		t.Fatalf("RandSeq(0) = %q", s)
	}
}

func TestRandInt63(t *testing.T) {
	seen := make(map[int64]bool)
	for i := 0; i < 1000; i++ {
		n := RandInt63()
		if n < 0 {
			t.Fatalf("negative: %d", n)
		}
		if seen[n] {
			t.Fatalf("%d drawn twice", n)
		}
		seen[n] = true
	}
}
