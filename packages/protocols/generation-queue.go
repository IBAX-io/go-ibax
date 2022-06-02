/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package protocols

import (
	"errors"
	"time"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
)

// BlockTimeChecker allow check queue to generate current block
type BlockTimeChecker interface {
	TimeToGenerate(position int64) (bool, error)
	BlockForTimeExists(t time.Time, nodePosition int) (bool, error)
	RangeByTime(at time.Time) (start, end time.Time, err error)
}

var (
	WrongNodePositionError = errors.New("wrong node position")
	TimeError              = errors.New("current time before first block")
	DuplicateBlockError    = errors.New("block for this time interval exists")
)

type BlockTimeCounter struct {
	start       time.Time
	duration    time.Duration
	numberNodes int
}

// Queue returns serial queue number for time
func (btc *BlockTimeCounter) queue(t time.Time) (int, error) {
	ut := t.Unix()
	t = time.Unix(ut, 0)
	if t.Before(btc.start) {
		return -1, TimeError
	}

	return int((t.Sub(btc.start) - 1) / btc.duration), nil
}

// NodePosition returns generating node position for time
func (btc *BlockTimeCounter) nodePosition(t time.Time) (int, error) {
	queue, err := btc.queue(t)
	if err != nil {
		return -1, err
	}

	return queue % btc.numberNodes, nil
}

func (btc *BlockTimeCounter) NodeTimeExists(t time.Time, nodePosition int) (bool, error) {
	ps, err := btc.nodePosition(t)
	if err != nil {
		return false, err
	}
	if ps == nodePosition {
		return true, nil
	}

	//startInterval, endInterval, err := btc.RangeByTime(t)
	//if err != nil {
	//	return false, err
	//}

	return false, nil
}

// BlockForTimeExists checks conformity between time and nodePosition
// changes functionality of ValidateBlock prevent blockTimeCalculator
func (btc *BlockTimeCounter) BlockForTimeExists(t time.Time, nodePosition int) (bool, error) {
	startInterval, endInterval, err := btc.RangeByTime(t)
	if err != nil {
		return false, err
	}

	b := &sqldb.BlockChain{}
	blocks, err := b.GetNodeBlocksAtTime(startInterval, endInterval, int64(nodePosition))
	if err != nil {
		return false, err
	}

	return len(blocks) > 0, nil
}

// NextTime returns next generation time for node position at time
func (btc *BlockTimeCounter) nextTime(t time.Time, nodePosition int) (time.Time, error) {
	if nodePosition >= btc.numberNodes {
		return time.Unix(0, 0), WrongNodePositionError
	}

	queue, err := btc.queue(t)
	if err != nil {
		return time.Unix(0, 0), err
	}
	curNodePosition := queue % btc.numberNodes

	d := nodePosition - curNodePosition
	if curNodePosition >= nodePosition {
		d += btc.numberNodes
	}

	return btc.start.Add(btc.duration*time.Duration(queue+d) + time.Millisecond), nil
}

// RangeByTime returns start and end of interval by time
func (btc *BlockTimeCounter) RangeByTime(t time.Time) (start, end time.Time, err error) {
	queue, err := btc.queue(t)
	if err != nil {
		st := time.Unix(0, 0)
		return st, st, err
	}

	start = btc.start.Add(btc.duration*time.Duration(queue) + time.Second)
	end = start.Add(btc.duration - time.Second)
	return
}

// TimeToGenerate returns true if the generation queue at time belongs to the specified node
func (btc *BlockTimeCounter) TimeToGenerate(at time.Time, nodePosition int) (bool, error) {
	if nodePosition >= btc.numberNodes {
		return false, WrongNodePositionError
	}

	position, err := btc.nodePosition(at)
	return position == nodePosition, err
}

// NewBlockTimeCounter return initialized BlockTimeCounter
func NewBlockTimeCounter() *BlockTimeCounter {
	btc := BlockTimeCounter{
		start:       time.Unix(syspar.GetFirstBlockTimestamp(), 0),
		duration:    syspar.GetMaxBlockTimeDuration(),
		numberNodes: int(syspar.GetCountOfActiveNodes()),
	}
	return &btc
}
