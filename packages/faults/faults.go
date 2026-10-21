/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package faults injects the faults of a network and of a clock into a node of a local test chain
// (F18 design 8, items 5, 6, 7 and 14): the partition of the validators, the latency of a WAN, a
// clock ahead. Only a build with the tag faults injects them; any other build, a release among
// them, reads the clock and dials as the standard library does.
//
// A node of the build reads File in its data directory every Poll; without the file it injects
// nothing. The file is a Config in JSON.
package faults

import "time"

// File is the name of the file of the faults in the data directory of a node
const File = "faults.json"

// Poll is how often a node reads the file of the faults
const Poll = 200 * time.Millisecond

// Config is the faults a node injects
type Config struct {
	// Refuse is the TCP addresses of the nodes the node does not reach: it dials none of them and
	// closes the connections it dialed to them. A partition is the nodes of each side refusing
	// the addresses of the other side, so no connection crosses it.
	Refuse []string `json:"refuse"`
	// DelayMs and JitterMs delay each write to another node by DelayMs, ± up to JitterMs, in the
	// order of the writes: on both ends of a connection, a round trip of 2 × DelayMs
	DelayMs  int `json:"delay_ms"`
	JitterMs int `json:"jitter_ms"`
	// ClockOffsetMs is how far ahead of the system clock Now is
	ClockOffsetMs int `json:"clock_offset_ms"`
}
