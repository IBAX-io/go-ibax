/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

// testPaceSet is a validators set of n keys "k0", "k1", …
func testPaceSet(t *testing.T, n int) (bft.Validators, [][]byte) {
	t.Helper()
	keys := make([][]byte, n)
	for i := range keys {
		keys[i] = []byte{'k', byte('0' + i)}
	}
	return bft.Validators{Keys: keys}, keys
}

// The proposer sizes to the slowest validator that said its execution lately, not to one far
// slower or silent or out of the set; to bftPaceDrain of it while one is more than BftExecDepth
// blocks behind; and an older execution status does not replace a newer one
func TestBftPaceRate(t *testing.T) {
	set, keys := testPaceSet(t, 4)
	now := time.Now()
	p := &bftPace{}
	const committed = 950

	rate, behind := p.rate(set, keys[0], 2000, committed, now)
	require.Equal(t, 2000.0, rate, "no execution status: the rate of the proposer")
	require.False(t, behind)

	p.take(keys[1], committed-1, 2100, now)
	p.take(keys[2], committed-2, 1850, now)
	rate, behind = p.rate(set, keys[0], 2000, committed, now)
	require.Equal(t, 1850.0, rate, "the slowest validator")
	require.False(t, behind, "BftExecDepth behind is not behind")

	p.take(keys[3], committed-5, 900, now)
	rate, _ = p.rate(set, keys[0], 2000, committed, now)
	require.Equal(t, 1850.0, rate, "a validator slower than 1/bftPaceSpread of the proposer does not count")

	p.take(keys[3], committed-consts.BftExecDepth-1, 1200, now)
	rate, behind = p.rate(set, keys[0], 2000, committed, now)
	require.True(t, behind)
	require.Equal(t, 1200*bftPaceDrain, rate, "a validator behind: bftPaceDrain of the slowest")

	p.take(keys[3], committed-10, 100, now)
	require.False(t, p.newer(keys[3], committed-10))
	require.True(t, p.newer(keys[3], committed))
	rate, _ = p.rate(set, keys[0], 2000, committed, now)
	require.Equal(t, 1200*bftPaceDrain, rate, "an older execution status replaced a newer one")

	rate, behind = p.rate(set, keys[0], 2000, committed, now.Add(bftPaceFresh+time.Second))
	require.Equal(t, 2000.0, rate, "execution statuses older than bftPaceFresh count")
	require.False(t, behind)

	p.take(keys[0], committed, 10, now)
	other, _ := testPaceSet(t, 2)
	p2 := &bftPace{}
	p2.take(keys[0], committed, 1500, now)
	p2.take(keys[3], committed, 1100, now)
	rate, _ = p2.rate(other, keys[1], 2000, committed, now)
	require.Equal(t, 1500.0, rate, "a key out of the set counts")
	rate, _ = p2.rate(other, keys[0], 2000, committed, now)
	require.Equal(t, 2000.0, rate, "the proposer's own execution status counts")
}

// The capacity is the pace rate for syspar.BftExecTarget, at least bftRateMin, at most
// max_tx_block; the gauge tells the rate the proposal was sized to, the counter the proposals
// sized down for a validator behind
func TestBftExecutorCapacity(t *testing.T) {
	require.Equal(t, 1500, capacityOf(1500, 5000))
	require.Equal(t, bftRateMin, capacityOf(20, 5000))
	require.Equal(t, 5000, capacityOf(9000, 5000))

	set, keys := testPaceSet(t, 4)
	e := testExecutor(10)
	e.rate = 2000
	e.pace.take(keys[2], 9, 1500, time.Now())
	e.capacity(set, keys[0], 10)
	m := &dto.Metric{}
	require.NoError(t, metrics.BftPaceRate.Write(m))
	require.Equal(t, 1500.0, m.GetGauge().GetValue())

	require.NoError(t, metrics.BftPaceBehind.Write(m))
	before := m.GetCounter().GetValue()
	e.pace.take(keys[3], 10-consts.BftExecDepth-1, 1800, time.Now())
	e.capacity(set, keys[0], 10)
	require.NoError(t, metrics.BftPaceBehind.Write(m))
	require.Equal(t, before+1, m.GetCounter().GetValue())
	require.NoError(t, metrics.BftPaceRate.Write(m))
	require.Equal(t, 1500*bftPaceDrain, m.GetGauge().GetValue())
}
