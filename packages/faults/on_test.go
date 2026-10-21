//go:build faults

/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package faults

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFaults sets the faults by the file: a refused address is not dialed and a connection to it
// closes, the writes of a connection come in their order after the delay, a connection closed
// writes what it took, and the clock is ahead by the offset
func TestFaults(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	got := make(chan []byte, 4)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				b, _ := io.ReadAll(c)
				got <- b
			}()
		}
	}()
	addr := l.Addr().String()
	path := filepath.Join(t.TempDir(), File)
	set := func(c Config) {
		b, _ := json.Marshal(c)
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(3 * Poll)
	}
	Watch(path)

	set(Config{DelayMs: 100, JitterMs: 20, ClockOffsetMs: 10000})
	if d := time.Until(Now()); d < 9*time.Second || d > 11*time.Second {
		t.Fatalf("the clock is %v ahead, want 10 s", d)
	}
	c, err := Dial(addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for _, s := range []string{"a", "b", "c"} {
		if _, err := c.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	c.Close()
	b := <-got
	if string(b) != "abc" {
		t.Fatalf("got %q, want the writes in their order", b)
	}
	if d := time.Since(start); d < 80*time.Millisecond {
		t.Fatalf("the writes came in %v, before the delay", d)
	}

	set(Config{})
	c, err = Dial(addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	set(Config{Refuse: []string{addr}})
	if _, err := Dial(addr, time.Second); err != ErrRefused {
		t.Fatalf("the dial of a refused address: %v", err)
	}
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil || os.IsTimeout(err) {
		t.Fatalf("the connection to a refused address stays open: %v", err)
	}
	set(Config{})
	if c, err = Dial(addr, time.Second); err != nil {
		t.Fatalf("the dial once the address is no more refused: %v", err)
	}
	c.Close()
}
