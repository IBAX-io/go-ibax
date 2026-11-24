/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"

	dto "github.com/prometheus/client_model/go"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func testExecutor(executed int64) *bftExecutor {
	e := &bftExecutor{logger: log.NewEntry(log.New()), wake: make(chan struct{}, 1), progress: make(chan struct{})}
	e.moved(executed)
	return e
}

// wait returns once the executor executed the height, at once for a height it executed, and
// ErrNotExecuted at the deadline; the executed height never moves back
func TestBftExecutorWait(t *testing.T) {
	e := testExecutor(5)
	ctx := context.Background()
	require.NoError(t, e.wait(ctx, 5, time.Now()))
	require.NoError(t, e.wait(ctx, 3, time.Now()))

	err := e.wait(ctx, 6, time.Now().Add(20*time.Millisecond))
	require.ErrorIs(t, err, block.ErrNotExecuted)

	done := make(chan error, 1)
	go func() { done <- e.wait(ctx, 7, time.Now().Add(5*time.Second)) }()
	e.moved(6)
	select {
	case err := <-done:
		t.Fatalf("the wait for 7 ended at 6: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	e.moved(7)
	require.NoError(t, <-done)

	e.moved(4)
	require.NoError(t, e.wait(ctx, 7, time.Now()), "the executed height moved back")

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, e.wait(cancelled, 8, time.Now().Add(time.Second)), context.Canceled)
}

// An execution hash of the chain that is not that of the node stops the executor and ends the
// waits with it; another failure is tried again after bftExecRetry
func TestBftExecutorFailed(t *testing.T) {
	e := testExecutor(5)
	ctx := context.Background()
	e.failed(6, errors.New("the database"))
	require.Nil(t, e.stopped)
	require.True(t, e.retryAt.After(time.Now().Add(bftExecRetry-time.Second)))
	require.False(t, e.step(), "the executor tries again before bftExecRetry")

	done := make(chan error, 1)
	go func() { done <- e.wait(ctx, 6, time.Now().Add(5*time.Second)) }()
	time.Sleep(10 * time.Millisecond)
	e.failed(6, fmt.Errorf("block 6: %w", block.ErrExecHash))
	require.ErrorIs(t, <-done, block.ErrExecHash)
	require.ErrorIs(t, e.wait(ctx, 6, time.Now().Add(time.Second)), block.ErrExecHash)
	e.retryAt = time.Time{}
	require.False(t, e.step(), "a stopped executor executes")
}

// idleSamples is the count and the sum of metrics.BftExecIdle
func idleSamples(t *testing.T) (uint64, float64) {
	m := &dto.Metric{}
	require.NoError(t, metrics.BftExecIdle.Write(m))
	return m.GetHistogram().GetSampleCount(), m.GetHistogram().GetSampleSum()
}

// The idle time of the executor runs from the commit of a block, or the end of the execution
// before it if that came later, to the start of its execution; nothing is observed for the first
// execution of the node nor for a block committed before it started
func TestBftExecutorIdle(t *testing.T) {
	e := testExecutor(5)
	now := time.Now()
	e.signal(6)
	e.idle(6, now)
	n, sum0 := idleSamples(t)
	e.lastEnd = now
	e.idle(7, now.Add(time.Second))
	got, _ := idleSamples(t)
	require.Equal(t, n, got, "an idle time of a block committed before the executor started")

	e.signal(8)
	e.signal(9)
	e.committedAt[8] = now.Add(-time.Second)
	e.idle(8, now.Add(300*time.Millisecond))
	got, sum := idleSamples(t)
	require.Equal(t, n+1, got)
	require.InDelta(t, 0.3, sum-sum0, 1e-9, "from the end of the execution before, after the commit")
	require.NotContains(t, e.committedAt, int64(8))
	require.Contains(t, e.committedAt, int64(9))

	e.committedAt[9] = now.Add(time.Second)
	e.idle(9, now.Add(1500*time.Millisecond))
	got, sum2 := idleSamples(t)
	require.Equal(t, n+2, got)
	require.InDelta(t, 0.5, sum2-sum, 1e-9, "from the commit, after the end of the execution before")
	require.Empty(t, e.committedAt)
}

// A node that synchronizes from height 1 commits a block by its certificate once its executor
// executed the block BftSetDelay before it, whose execution stored the validators set of the
// height: the certificates of heights 1 to BftSetDelay are checked at once, that of the next height
// waits for the execution of block 1
func TestBftSyncWaitsForExecutor(t *testing.T) {
	saved := bftExec
	bftExec = testExecutor(0)
	t.Cleanup(func() { bftExec = saved })
	ctx := context.Background()
	for h := uint64(1); h <= consts.BftSetDelay; h++ {
		err := commitSynced(ctx, h, []byte{0xff}, nil)
		require.Error(t, err)
		require.NotErrorIs(t, err, block.ErrNotExecuted, "height %d waits for the executor", h)
	}

	done := make(chan error, 1)
	go func() { done <- commitSynced(ctx, consts.BftSetDelay+1, []byte{0xff}, nil) }()
	select {
	case err := <-done:
		t.Fatalf("the certificate of height %d is checked before block 1 is executed: %v", consts.BftSetDelay+1, err)
	case <-time.After(50 * time.Millisecond):
	}
	bftExec.moved(1)
	err := <-done
	require.Error(t, err, "the certificate does not decode")
	require.NotErrorIs(t, err, block.ErrNotExecuted)

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, commitSynced(cancelled, consts.BftSetDelay+2, nil, nil), context.Canceled)
}
