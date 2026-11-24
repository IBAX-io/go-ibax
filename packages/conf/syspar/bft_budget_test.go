/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package syspar

import (
	"testing"
	"time"
)

// The budget of a proposer is bft_timeout_propose_ms less BftProposalMargin, and grows with the
// round as the timeout does; max_block_generation_time takes no part
func TestBftProposeBudget(t *testing.T) {
	mutex.Lock()
	saved := cache
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		cache = saved
		mutex.Unlock()
	}()
	set := func(block, propose string) {
		mutex.Lock()
		cache = map[string]string{MaxBlockGenerationTime: block}
		for _, p := range BftParameters {
			cache[p[0]] = p[1]
		}
		cache[BftTimeoutPropose] = propose
		mutex.Unlock()
	}
	for _, c := range []struct {
		block, propose string
		round          uint32
		want           time.Duration
	}{
		{"2000", "4000", 0, 3500 * time.Millisecond},
		{"2000", "4000", 1, 4000 * time.Millisecond},
		{"2000", "4000", 4, 5500 * time.Millisecond},
		{"500", "4000", 0, 3500 * time.Millisecond},
		{"9000", "2600", 0, 2100 * time.Millisecond},
		{"9000", "2600", 2, 3100 * time.Millisecond},
	} {
		set(c.block, c.propose)
		propose, _, _ := GetBftTimeouts()
		got := GetBftProposeBudget(c.round)
		if got != c.want {
			t.Errorf("block %s ms, propose %s ms, round %d: budget %v, want %v", c.block, c.propose, c.round, got, c.want)
		}
		if timeout := propose + time.Duration(c.round)*BftTimeoutDelta; got > timeout-BftProposalMargin {
			t.Errorf("block %s ms, propose %s ms, round %d: budget %v leaves less than the margin of the timeout %v", c.block, c.propose, c.round, got, timeout)
		}
	}
}
