/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package protocols

import (
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockTimeCalculator_TimeToGenerate(t *testing.T) {
	cases := []struct {
		firstBlockTime time.Time
		blockGenTime   time.Duration
		blocksGap      time.Duration
		nodesCount     int64
		clock          utils.Clock
		blocksCounter  intervalBlocksCounter
		nodePosition   int64

		result bool
		err    error
	}{
		{
			firstBlockTime: time.Unix(1, 0),

			clock: func() utils.Clock {
				mc := &utils.MockClock{}
				mc.On("Now").Return(time.Unix(0, 0))
				return mc
			}(),

			err: TimeError,
		},

		{
			firstBlockTime: time.Unix(1, 0),
			blockGenTime:   time.Second * 2,
			blocksGap:      time.Second * 3,
			nodesCount:     3,
			nodePosition:   2,

			clock: func() utils.Clock {
				mc := &utils.MockClock{}
				mc.On("Now").Return(time.Unix(16, 0))
				return mc
			}(),
			blocksCounter: func() intervalBlocksCounter {
				ibc := &mockIntervalBlocksCounter{}
				ibc.On("count", blockGenerationState{
					start:        time.Unix(13, 0),
					duration:     time.Second * 5,
					nodePosition: 2,
				}).Return(1, nil)
				return ibc
			}(),

			result: false,
			err:    DuplicateBlockError,
		},

		{
			firstBlockTime: time.Unix(1, 0),
			blockGenTime:   time.Second * 2,
			blocksGap:      time.Second * 3,
			nodesCount:     3,
			nodePosition:   2,

			clock: func() utils.Clock {
				mc := &utils.MockClock{}
				mc.On("Now").Return(time.Unix(16, 0))
				return mc
			}(),
			blocksCounter: func() intervalBlocksCounter {
				ibc := &mockIntervalBlocksCounter{}
				ibc.On("count", blockGenerationState{
					start:        time.Unix(13, 0),
					duration:     time.Second * 5,
					nodePosition: 2,
				}).Return(0, nil)
				return ibc
			}(),

			result: true,
		},
	}

	for _, c := range cases {
		btc := NewBlockTimeCalculator(c.firstBlockTime,
			c.blockGenTime,
			c.blocksGap,
			c.nodesCount,
		)

		execResult, execErr := btc.
			SetClock(c.clock).
			setBlockCounter(c.blocksCounter).
			TimeToGenerate(c.nodePosition)

		require.Equal(t, c.err, execErr)
		assert.Equal(t, c.result, execResult)
	}
}

func TestBlockTimeCalculator_ValidateBlock(t *testing.T) {
	cases := []struct {
		firstBlockTime time.Time
		blockGenTime   time.Duration
		blocksGap      time.Duration
		nodesCount     int64
		time           time.Time
		blocksCounter  intervalBlocksCounter
		nodePosition   int64

		result bool
		err    error
	}{
		{
			firstBlockTime: time.Unix(1, 0),
			time:           time.Unix(0, 0),

			err: TimeError,
		},

		{
			firstBlockTime: time.Unix(1, 0),
			blockGenTime:   time.Second * 2,
			blocksGap:      time.Second * 3,
			nodesCount:     3,
			nodePosition:   2,

			time: time.Unix(16, 0),
			blocksCounter: func() intervalBlocksCounter {
				ibc := &mockIntervalBlocksCounter{}
				ibc.On("count", blockGenerationState{
					start:        time.Unix(13, 0),
					duration:     time.Second * 5,
					nodePosition: 2,
				}).Return(1, nil)
				return ibc
			}(),

			result: false,
			err:    DuplicateBlockError,
		},

		{
			firstBlockTime: time.Unix(1, 0),
			blockGenTime:   time.Second * 2,
			blocksGap:      time.Second * 3,
			nodesCount:     3,
			nodePosition:   2,

			time: time.Unix(16, 0),
			blocksCounter: func() intervalBlocksCounter {
				ibc := &mockIntervalBlocksCounter{}
				ibc.On("count", blockGenerationState{
					start:        time.Unix(13, 0),
					duration:     time.Second * 5,
					nodePosition: 2,
				}).Return(0, nil)
				return ibc
			}(),

			result: true,
		},
	}

	for _, c := range cases {
		btc := NewBlockTimeCalculator(c.firstBlockTime,
			c.blockGenTime,
			c.blocksGap,
			c.nodesCount,
		)

		execResult, execErr := btc.
			setBlockCounter(c.blocksCounter).
			ValidateBlock(c.nodePosition, c.time)

		require.Equal(t, c.err, execErr)
		assert.Equal(t, c.result, execResult)
	}
}

func TestBlockTImeCalculator_countBlockTime(t *testing.T) {
	cases := []struct {
		firstBlockTime time.Time
		blockGenTime   time.Duration
		blocksGap      time.Duration
		nodesCount     int64
		clock          time.Time

		result blockGenerationState
		err    error
	}{
		// Current time before first block case
		{
			firstBlockTime: time.Unix(1, 0),
			clock:          time.Unix(0, 0),

			err: TimeError,
		},

		// Zero duration case
		{
			firstBlockTime: time.Unix(0, 0),
			blockGenTime:   time.Second * 0,
			blocksGap:      time.Second * 0,
			nodesCount:     5,
			clock:          time.Unix(0, 0),

			result: blockGenerationState{
				start:    time.Unix(0, 0),
				duration: time.Second * 0,

				nodePosition: 0,
			},
		},

		// Duration testing case
		{
			firstBlockTime: time.Unix(0, 0),
			blockGenTime:   time.Second * 1,
			blocksGap:      time.Second * 0,
			nodesCount:     5,
			clock:          time.Unix(0, 0),

			result: blockGenerationState{
				start:    time.Unix(0, 0),
				duration: time.Second * 1,

				nodePosition: 0,
			},
		},

		// Duration testing case
		{
			firstBlockTime: time.Unix(0, 0),
			blockGenTime:   time.Second * 0,
			blocksGap:      time.Second * 1,
			nodesCount:     5,
			clock:          time.Unix(0, 0),

			result: blockGenerationState{
				start:    time.Unix(0, 0),
				duration: time.Second * 1,

				nodePosition: 0,
			},
		},

		// Duration testing case
		{
			firstBlockTime: time.Unix(0, 0),
			blockGenTime:   time.Second * 4,
			blocksGap:      time.Second * 6,
			nodesCount:     5,
			clock:          time.Unix(0, 0),

			result: blockGenerationState{
				start:    time.Unix(0, 0),
				duration: time.Second * 10,

				nodePosition: 0,
			},
		},

		// Block lowest time boundary case
		{
			firstBlockTime: time.Unix(0, 0),
			blockGenTime:   time.Second * 1,
			blocksGap:      time.Second * 1,
			nodesCount:     10,
			clock:          time.Unix(0, 0),

			result: blockGenerationState{
				start:    time.Unix(0, 0),
				duration: time.Second * 2,

				nodePosition: 0,
			},
		},

		// Block highest time boundary case
		{
			firstBlockTime: time.Unix(0, 0),
			blockGenTime:   time.Second * 2,
			blocksGap:      time.Second * 3,
			nodesCount:     10,
			clock:          time.Unix(5, 999999999),

			result: blockGenerationState{
				start:    time.Unix(0, 0),
				duration: time.Second * 5,

				nodePosition: 0,
			},
		},

		// Last nodePosition case
		{
			firstBlockTime: time.Unix(0, 0),
			blockGenTime:   time.Second * 0,
			blocksGap:      time.Second * 1,
			nodesCount:     3,
			clock:          time.Unix(6, 0),

			result: blockGenerationState{
				start:    time.Unix(6, 0),
				duration: time.Second * 1,

				nodePosition: 0,
			},
		},

		// One node case
		{
			firstBlockTime: time.Unix(0, 0),
			blockGenTime:   time.Second * 2,
			blocksGap:      time.Second * 2,
			nodesCount:     1,
			clock:          time.Unix(6, 0),

			result: blockGenerationState{
				start:    time.Unix(5, 0),
				duration: time.Second * 4,

				nodePosition: 0,
			},
		},

		// Custom firstBlockTime case
		{
			firstBlockTime: time.Unix(1, 0),
			blockGenTime:   time.Second * 2,
			blocksGap:      time.Second * 3,
			nodesCount:     3,
			clock:          time.Unix(13, 0),

			result: blockGenerationState{
				start:    time.Unix(13, 0),
				duration: time.Second * 5,

				nodePosition: 2,
			},
		},

		// Current time is in middle of interval case
		{
			firstBlockTime: time.Unix(1, 0),
			blockGenTime:   time.Second * 2,
			blocksGap:      time.Second * 3,
			nodesCount:     3,
			clock:          time.Unix(16, 0),

			result: blockGenerationState{
				start:    time.Unix(13, 0),
				duration: time.Second * 5,

				nodePosition: 2,
			},
		},

		// Real life case
		{
			firstBlockTime: time.Unix(1519240000, 0),
			blockGenTime:   time.Second * 4,
			blocksGap:      time.Second * 5,
			nodesCount:     101,
			clock:          time.Unix(1519241010, 1234),

			result: blockGenerationState{
				start:    time.Unix(1519241010, 0),
				duration: time.Second * 9,

				nodePosition: 0,
			},
		},
	}

	for _, c := range cases {
		btc := NewBlockTimeCalculator(c.firstBlockTime,
			c.blockGenTime,
			c.blocksGap,
			c.nodesCount,
		)

		execResult, execErr := btc.countBlockTime(c.clock)
		require.Equal(t, c.err, execErr)
		assert.Equal(t, c.result, execResult)
	}
}
