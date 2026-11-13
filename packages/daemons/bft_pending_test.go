/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

// pendingTest is a bftPending of a set of 4 whose node is validator 0, on a clock the test moves
type pendingTest struct {
	p *bftPending

	mu      sync.Mutex
	now     time.Time
	pool    int
	have    map[string]int // the transactions each validator gives, by its key
	fetched []string       // the keys pulled from
	timeout []time.Duration
	works   chan [2]any
	said    [][2]uint64
}

func newPendingTest(t *testing.T) *pendingTest {
	pt := &pendingTest{now: time.Unix(1000, 0), have: map[string]int{}, works: make(chan [2]any, 16)}
	pt.p = &bftPending{
		poolLen: func() int { pt.mu.Lock(); defer pt.mu.Unlock(); return pt.pool },
		fetch: func(key []byte, timeout time.Duration) ([][]byte, error) {
			pt.mu.Lock()
			defer pt.mu.Unlock()
			pt.fetched = append(pt.fetched, string(key))
			pt.timeout = append(pt.timeout, timeout)
			n, ok := pt.have[string(key)]
			if !ok {
				return nil, errors.New("unreachable")
			}
			return make([][]byte, n), nil
		},
		receive: func(bodies [][]byte) int {
			pt.mu.Lock()
			defer pt.mu.Unlock()
			pt.pool += len(bodies)
			return len(bodies)
		},
		work: func(height uint64, at time.Time) { pt.works <- [2]any{height, at} },
		say: func(height uint64, validator uint32) {
			pt.mu.Lock()
			defer pt.mu.Unlock()
			pt.said = append(pt.said, [2]uint64{height, uint64(validator)})
		},
		timeout: 5 * time.Second,
		now:     func() time.Time { pt.mu.Lock(); defer pt.mu.Unlock(); return pt.now },
	}
	pt.p.moveTo(288, alertSet(4), alertSet(4).Keys[0])
	return pt
}

func (pt *pendingTest) advance(d time.Duration) { pt.mu.Lock(); pt.now = pt.now.Add(d); pt.mu.Unlock() }

func (pt *pendingTest) set(f func()) { pt.mu.Lock(); f(); pt.mu.Unlock() }

// pulls is the keys pulled from once no pull runs
func (pt *pendingTest) pulls(t *testing.T) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		pt.p.mu.Lock()
		busy := pt.p.busy
		pt.p.mu.Unlock()
		if !busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a pull runs on")
		}
		time.Sleep(time.Millisecond)
	}
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return slices.Clone(pt.fetched)
}

func (pt *pendingTest) work(t *testing.T) [2]any {
	t.Helper()
	select {
	case w := <-pt.works:
		return w
	case <-time.After(5 * time.Second):
		t.Fatal("no work")
	}
	return [2]any{}
}

const key3, key2, key1 = "validator 3", "validator 2", "validator 1"

// A node with an empty pool pulls the transactions of a validator that says it has some at the
// height and starts the height from when it said so; the words of the node itself, of no
// validator of the set and of other heights pull nothing
func TestBftPendingPullsWhenEmpty(t *testing.T) {
	pt := newPendingTest(t)
	pt.set(func() { pt.have[key3] = 5 })
	pt.p.hint(288, 0)
	pt.p.hint(288, 9)
	pt.p.hint(287, 3)
	pt.p.hint(290, 3)
	if got := pt.pulls(t); len(got) != 0 {
		t.Fatalf("pulled from %q", got)
	}
	at := pt.now
	pt.p.hint(288, 3)
	if w := pt.work(t); w[0] != uint64(288) || !w[1].(time.Time).Equal(at) {
		t.Fatalf("the work is %v, want 288 from %v", w, at)
	}
	if got := pt.pulls(t); !slices.Equal(got, []string{key3}) {
		t.Fatalf("pulled from %q", got)
	}
	// the pool has transactions now: another word pulls nothing
	pt.advance(3 * time.Second)
	pt.p.hint(288, 2)
	if got := pt.pulls(t); len(got) != 1 {
		t.Fatalf("pulled from %q with transactions", got)
	}
}

// A node with transactions pulls none
func TestBftPendingNotWithWork(t *testing.T) {
	pt := newPendingTest(t)
	pt.set(func() { pt.pool, pt.have[key3] = 1, 5 })
	pt.p.hint(288, 3)
	if got := pt.pulls(t); len(got) != 0 {
		t.Fatalf("pulled from %q", got)
	}
}

// A node pulls from a validator once a bftResend; a pull that takes nothing goes on to another
// validator that said so, the first to say it first
func TestBftPendingOncePerResend(t *testing.T) {
	pt := newPendingTest(t)
	pt.p.hint(288, 3)
	pt.advance(time.Millisecond)
	pt.p.hint(288, 1)
	pt.p.hint(288, 3)
	pt.p.hint(288, 1)
	if got := pt.pulls(t); !slices.Equal(got, []string{key3, key1}) {
		t.Fatalf("pulled from %q", got)
	}
	// past bftResend a word pulls again from both in turn
	pt.advance(bftResend)
	pt.set(func() { pt.have[key1] = 2 })
	pt.p.hint(288, 1)
	pt.work(t)
	if got := pt.pulls(t); !slices.Equal(got, []string{key3, key1, key3, key1}) {
		t.Fatalf("pulled from %q", got)
	}
}

// The words of the height after are kept and pulled from when the node moves to it; those of the
// height before are dropped
func TestBftPendingNextHeight(t *testing.T) {
	pt := newPendingTest(t)
	pt.set(func() { pt.have[key2], pt.have[key3] = 3, 3 })
	pt.p.hint(289, 2)
	if got := pt.pulls(t); len(got) != 0 {
		t.Fatalf("pulled from %q for the next height", got)
	}
	pt.p.moveTo(289, alertSet(4), alertSet(4).Keys[0])
	if w := pt.work(t); w[0] != uint64(289) {
		t.Fatalf("the work is %v", w)
	}
	if got := pt.pulls(t); !slices.Equal(got, []string{key2}) {
		t.Fatalf("pulled from %q", got)
	}
	pt.set(func() { pt.pool = 0 })
	pt.p.moveTo(290, alertSet(4), alertSet(4).Keys[0])
	if got := pt.pulls(t); len(got) != 1 {
		t.Fatalf("pulled from %q at a height of no word", got)
	}
}

// The proposer without transactions pulls at once from the validators that said they had some,
// within its deadline
func TestBftPendingPullNow(t *testing.T) {
	pt := newPendingTest(t)
	deadline := pt.now.Add(3 * time.Second)
	if pt.p.pullNow(288, deadline) {
		t.Fatal("pulled with no word")
	}
	pt.set(func() { pt.pool, pt.have[key2] = 1, 4 })
	pt.p.hint(289, 2)
	pt.p.hint(288, 3)
	pt.advance(time.Millisecond)
	pt.p.hint(288, 2)
	pt.set(func() { pt.pool = 0 })
	if pt.p.pullNow(289, deadline) {
		t.Fatal("pulled for another height")
	}
	// validator 3 is unreachable, validator 2 gives 4
	if !pt.p.pullNow(288, deadline) {
		t.Fatal("no transactions pulled")
	}
	pt.mu.Lock()
	fetched, timeout := slices.Clone(pt.fetched), slices.Clone(pt.timeout)
	pt.mu.Unlock()
	if !slices.Equal(fetched, []string{key3, key2}) || timeout[0] != 3*time.Second-time.Millisecond {
		t.Fatalf("pulled from %q in %v", fetched, timeout)
	}
	pt.advance(3 * time.Second)
	if pt.p.pullNow(288, deadline) {
		t.Fatal("pulled past the deadline")
	}
}

// A validator says it has transactions once a bftResend at its height; a node of no validator
// says nothing
func TestBftPendingAnnounce(t *testing.T) {
	pt := newPendingTest(t)
	pt.p.announce(288)
	pt.p.announce(288)
	pt.p.announce(289)
	pt.advance(bftResend)
	pt.p.announce(288)
	pt.p.moveTo(289, alertSet(4), alertSet(4).Keys[2])
	pt.p.announce(289)
	pt.p.moveTo(290, alertSet(4), []byte("no validator"))
	pt.p.announce(290)
	pt.mu.Lock()
	defer pt.mu.Unlock()
	want := [][2]uint64{{288, 0}, {288, 0}, {289, 2}}
	if !slices.Equal(pt.said, want) {
		t.Fatalf("said %v, want %v", pt.said, want)
	}
}
