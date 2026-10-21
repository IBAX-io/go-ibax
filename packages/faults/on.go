//go:build faults

/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package faults

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
)

// Enabled reports whether the build injects the faults
const Enabled = true

// ErrRefused is the dial of a node the faults refuse
var ErrRefused = errors.New("faults: the node is refused")

var (
	current atomic.Pointer[Config]
	watched sync.Once
	mu      sync.Mutex
	dialed  = map[*conn]struct{}{}
)

func config() *Config {
	if c := current.Load(); c != nil {
		return c
	}
	return &Config{}
}

// Watch reads the faults of the file at path from now on, every Poll
func Watch(path string) {
	watched.Do(func() {
		log.WithFields(log.Fields{"path": path}).Warn("the build injects the faults of the file")
		read := func() {
			c := &Config{}
			if b, err := os.ReadFile(path); err == nil {
				if err := json.Unmarshal(b, c); err != nil {
					log.WithFields(log.Fields{"path": path, "error": err}).Error("reading the faults")
					return
				}
			}
			if old := config(); !equal(old, c) {
				log.WithFields(log.Fields{"refuse": c.Refuse, "delay_ms": c.DelayMs, "jitter_ms": c.JitterMs,
					"clock_offset_ms": c.ClockOffsetMs}).Warn("the faults change")
			}
			current.Store(c)
			mu.Lock()
			for k := range dialed {
				if slices.Contains(c.Refuse, k.addr) {
					k.Conn.Close()
				}
			}
			mu.Unlock()
		}
		read()
		go func() {
			for range time.Tick(Poll) {
				read()
			}
		}()
	})
}

func equal(a, b *Config) bool {
	return slices.Equal(a.Refuse, b.Refuse) && a.DelayMs == b.DelayMs && a.JitterMs == b.JitterMs &&
		a.ClockOffsetMs == b.ClockOffsetMs
}

// Now is the clock of the node: the clock of the system, ClockOffsetMs ahead
func Now() time.Time {
	return time.Now().Add(time.Duration(config().ClockOffsetMs) * time.Millisecond)
}

// Dial dials another node unless the faults refuse its address
func Dial(addr string, timeout time.Duration) (net.Conn, error) {
	if slices.Contains(config().Refuse, addr) {
		return nil, ErrRefused
	}
	c, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	k := wrap(c, addr)
	mu.Lock()
	dialed[k] = struct{}{}
	mu.Unlock()
	return k, nil
}

// Accepted is a connection another node dialed, its writes delayed
func Accepted(c net.Conn) net.Conn { return wrap(c, "") }

type chunk struct {
	due  time.Time
	data []byte
}

// conn delays its writes: a writer writes each chunk at its time, in order
type conn struct {
	net.Conn
	addr   string
	mu     sync.Mutex
	closed bool
	last   time.Time
	queue  chan chunk
	err    atomic.Pointer[error]
}

func wrap(c net.Conn, addr string) *conn {
	k := &conn{Conn: c, addr: addr, queue: make(chan chunk, 4096)}
	go k.write()
	return k
}

func (k *conn) write() {
	defer func() {
		mu.Lock()
		delete(dialed, k)
		mu.Unlock()
		k.Conn.Close()
	}()
	for ch := range k.queue {
		if k.err.Load() != nil {
			continue
		}
		time.Sleep(time.Until(ch.due))
		if _, err := k.Conn.Write(ch.data); err != nil {
			k.err.Store(&err)
		}
	}
}

func (k *conn) Write(b []byte) (int, error) {
	if err := k.err.Load(); err != nil {
		return 0, *err
	}
	c := config()
	due := time.Now()
	if c.DelayMs > 0 {
		d := time.Duration(c.DelayMs) * time.Millisecond
		if c.JitterMs > 0 {
			d += time.Duration(rand.IntN(2*c.JitterMs+1)-c.JitterMs) * time.Millisecond
		}
		due = due.Add(d)
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.closed {
		return 0, net.ErrClosed
	}
	if due.Before(k.last) {
		due = k.last
	}
	k.last = due
	k.queue <- chunk{due, slices.Clone(b)}
	return len(b), nil
}

// Close closes the connection once the writer wrote the chunks before it
func (k *conn) Close() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.closed {
		k.closed = true
		close(k.queue)
	}
	return nil
}
