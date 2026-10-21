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

// Dial dials another node: net.DialTimeout
func Dial(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, timeout)
}

// Accepted is a connection another node dialed: conn
func Accepted(conn net.Conn) net.Conn { return conn }
