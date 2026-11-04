//go:build !faults

/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package faults

import (
	"net"
	"time"
)

// Enabled reports whether the build injects the faults
const Enabled = false

// Watch reads the faults of the file at path from now on
func Watch(path string) {}

// Now is the clock of the node: the clock of the system
func Now() time.Time { return time.Now() }

// Until is how long the clock of the node takes until t: time.Until
func Until(t time.Time) time.Duration { return time.Until(t) }

// Span is how far the clock of the node goes while the system clock goes d: d
func Span(d time.Duration) time.Duration { return d }

// Dial dials another node: net.DialTimeout
func Dial(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, timeout)
}

// Accepted is a connection another node dialed: conn
func Accepted(conn net.Conn) net.Conn { return conn }
