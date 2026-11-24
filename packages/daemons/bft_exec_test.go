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
