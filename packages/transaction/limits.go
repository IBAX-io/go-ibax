/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/script"
	log "github.com/sirupsen/logrus"
)

type LimitMode int

const (
	letPreprocess LimitMode = 0x0001 // checking before generating block
	letGenBlock   LimitMode = 0x0002 // checking during generating block
	letParsing    LimitMode = 0x0004 // common checking during parsing block
)

func GetLetPreprocess() LimitMode {
	return letPreprocess
}

func GetLetGenBlock() LimitMode {
	return letGenBlock
}

func GetLetParsing() LimitMode {
	return letParsing
}

// Limits is used for saving current limit information
type Limits struct {
	Mode     LimitMode
	Limiters []Limiter // the list of limiters
	// of is, for the limits of a run in parallel, the limits of the block: the run checks none,
	// the block checks them in its order once the run commits
	of *Limits
}

// Deferred are the limits of a run in parallel: they check nothing, and tell the time left to the
// block
func (limits *Limits) Deferred() *Limits {
	return &Limits{Mode: limits.Mode, of: limits}
}

// Limiter describes interface functions for limits
type Limiter interface {
	init()
	check(TransactionCaller, LimitMode) error
}

type limiterModes struct {
	limiter Limiter
	modes   LimitMode // combination of letPreprocess letGenBlock letParsing
}

var (
	// ErrLimitSkip returns when tx should be skipped during generating block
	ErrLimitSkip = errors.New(`skip tx`)
	// ErrLimitStop returns when the generation of the block should be stopped
	ErrLimitStop = errors.New(`stop generating block`)
	// ErrTimeLeft is ErrLimitStop for a transaction the time left to the block stopped
	ErrTimeLeft = errors.WithMessage(ErrLimitStop, "the block has no time left")
)

// NewLimits initializes Limits structure.
func NewLimits(b LimitMode) (limits *Limits) {
	limits = &Limits{Limiters: make([]Limiter, 0, 8)}

	limits.Mode = b

	allLimiters := []limiterModes{
		{limiter: &txMaxSize{}, modes: letPreprocess | letGenBlock | letParsing},
		{limiter: &txUserLimit{}, modes: letPreprocess | letParsing},
		{limiter: &txMaxLimit{}, modes: letPreprocess | letParsing},
		{limiter: &txUserEcosysLimit{}, modes: letPreprocess | letParsing},
		{limiter: &timeBlockLimit{}, modes: letGenBlock},
		{limiter: &txMaxFuel{}, modes: letGenBlock | letParsing},
	}
	for _, limiter := range allLimiters {
		if limiter.modes&limits.Mode == 0 {
			continue
		}
		limiter.limiter.init()
		limits.Limiters = append(limits.Limiters, limiter.limiter)
	}
	return
}

// CheckLimit calls each limiter
func (limits *Limits) CheckLimit(t TransactionCaller) error {
	if limits.of != nil {
		return nil
	}
	for _, limiter := range limits.Limiters {
		if err := limiter.check(t, limits.Mode); err != nil {
			return err
		}
	}
	return nil
}

func limitError(limitName, msg string, args ...any) error {
	err := fmt.Errorf(msg, args...)
	log.WithFields(log.Fields{"type": consts.BlockError, "error": err}).Error(limitName)
	return script.SetVMError(`panic`, err)
}

// Checking the max tx in the block
type txMaxLimit struct {
	Count int // the current count
	Limit int // max count of tx in the block
}

func (bl *txMaxLimit) init() {
	bl.Limit = syspar.GetMaxTxCount()
}

func (bl *txMaxLimit) check(t TransactionCaller, mode LimitMode) error {
	bl.Count++
	if bl.Count+1 > bl.Limit && mode == letPreprocess {
		return errors.WithMessage(ErrLimitStop, "txMaxLimit")
	}
	if bl.Count > bl.Limit {
		return limitError(`txMaxLimit`, `Max tx in the block`)
	}
	return nil
}

// Checking the time of the start of generating block: a transaction ends within the time of the
// block less what the block takes after its transactions run, Tail for each transaction it took
// and for the transaction, so the whole block is made within the time of the block
type timeBlockLimit struct {
	Start time.Time     // the time of the start of generating block
	Limit time.Duration // the maximum time
	// Tail is what a transaction the block took adds to the block after the transactions run:
	// writing its state, its rows and the block, and rolling the trial back
	Tail  time.Duration
	Count int // the transactions the block took
}

func (bl *timeBlockLimit) init() {
	bl.Start = time.Now()
	bl.Limit = time.Millisecond * time.Duration(syspar.GetMaxBlockGenerationTime())
}

// reserve is the time the block keeps for the tail of n transactions, at most half its time
func (bl *timeBlockLimit) reserve(n int) time.Duration {
	return min(bl.Tail*time.Duration(n), bl.Limit/2)
}

// left is the time left to the next transaction of the block. The first transaction has the whole
// time of the block, none kept for its tail: a transaction the time stops as the first of its block
// had the most time a block leaves a transaction, and the block takes it as one that never fits.
func (bl *timeBlockLimit) left() time.Duration {
	var reserve time.Duration
	if bl.Count > 0 {
		reserve = bl.reserve(bl.Count + 1)
	}
	return max(0, bl.Limit-reserve-max(0, time.Since(bl.Start)))
}

func (bl *timeBlockLimit) check(t TransactionCaller, mode LimitMode) error {
	if bl.left() > 0 {
		bl.Count++
		return nil
	}

	if mode == letGenBlock {
		return ErrTimeLeft
	}

	return limitError("txBlockTimeLimit", "Block generation time exceeded")
}

// StartAt starts the time of the block being generated at start, and keeps tail of it for each
// transaction the block takes
func (limits *Limits) StartAt(start time.Time, tail time.Duration) {
	for _, l := range limits.Limiters {
		if bl, ok := l.(*timeBlockLimit); ok {
			bl.Start, bl.Tail = start, tail
		}
	}
}

// TimeLeft is the time left to the next transaction of the block being generated, in ms, at most
// max_block_generation_time
func (limits *Limits) TimeLeft() int64 {
	if limits.of != nil {
		return limits.of.TimeLeft()
	}
	for _, l := range limits.Limiters {
		if bl, ok := l.(*timeBlockLimit); ok {
			return bl.left().Milliseconds()
		}
	}
	return syspar.GetMaxBlockGenerationTime()
}

// Checking the max tx from one user in the block
type txUserLimit struct {
	TxUsers map[int64]int // the counter of tx from one user
	Limit   int           // the value of max tx from one user
}

func (bl *txUserLimit) init() {
	bl.TxUsers = make(map[int64]int)
	bl.Limit = syspar.GetMaxBlockUserTx()
}

func (bl *txUserLimit) check(t TransactionCaller, mode LimitMode) error {
	var (
		count int
		ok    bool
	)
	keyID := t.txKeyID()
	if count, ok = bl.TxUsers[keyID]; ok {
		if count+1 > bl.Limit && mode == letPreprocess {
			return ErrLimitSkip
		}
		if count > bl.Limit {
			return limitError(`txUserLimit`, `Max tx from one user %d`, keyID)
		}
	}
	bl.TxUsers[keyID] = count + 1
	return nil
}

// Checking the max tx from one user in the ecosystem contracts
type ecosysLimit struct {
	TxUsers map[int64]int // the counter of tx from one user in the ecosystem
	Limit   int           // the value of max tx from one user in the ecosystem
}

type txUserEcosysLimit struct {
	TxEcosys map[int64]ecosysLimit // the counter of tx from one user in ecosystems
}

func (bl *txUserEcosysLimit) init() {
	bl.TxEcosys = make(map[int64]ecosysLimit)
}

func (bl *txUserEcosysLimit) check(t TransactionCaller, mode LimitMode) error {
	keyID := t.txKeyID()
	smart, ok := t.(*SmartTransactionParser)
	if !ok {
		return nil
	}
	ecosystemID := smart.TxSmart.EcosystemID
	if val, ok := bl.TxEcosys[ecosystemID]; ok {
		if user, ok := val.TxUsers[keyID]; ok {
			if user+1 > val.Limit && mode == letPreprocess {
				return ErrLimitSkip
			}
			if user > val.Limit {
				return limitError(`txUserEcosysLimit`, `Max tx from one user %d in ecosystem %d`,
					keyID, ecosystemID)
			}
			val.TxUsers[keyID] = user + 1
		} else {
			val.TxUsers[keyID] = 1
		}
	} else {
		limit := syspar.GetMaxBlockUserTx()
		bl.TxEcosys[ecosystemID] = ecosysLimit{TxUsers: make(map[int64]int), Limit: limit}
		bl.TxEcosys[ecosystemID].TxUsers[keyID] = 1
	}
	return nil
}

// Checking the max tx size. The block size is checked on the encoded block: the node generating
// it counts it as its transactions run (block.blockSize), and its peers check the bytes they get.
type txMaxSize struct {
	LimitTx int64 // max size of tx
}

func (bl *txMaxSize) init() {
	bl.LimitTx = syspar.GetMaxTxSize()
}

func (bl *txMaxSize) check(t TransactionCaller, mode LimitMode) error {
	if t.txSize() > bl.LimitTx {
		return limitError(`txMaxSize`, `Max size of tx`)
	}
	return nil
}

// Checking the max tx & block size
type txMaxFuel struct {
	Fuel       int64 // the current fuel of the block
	LimitBlock int64 // max fuel of the block
	LimitTx    int64 // max fuel of tx
}

func (bl *txMaxFuel) init() {
	bl.LimitBlock = syspar.GetMaxBlockFuel()
	bl.LimitTx = syspar.GetMaxTxFuel()
}

func (bl *txMaxFuel) check(t TransactionCaller, mode LimitMode) error {
	smart, ok := t.(*SmartTransactionParser)
	if !ok {
		return nil
	}
	fuel := smart.TxFuel
	if fuel > bl.LimitTx {
		return limitError(`txMaxFuel`, `Max fuel of tx %d > %d`, fuel, bl.LimitTx)
	}
	bl.Fuel += fuel
	if bl.Fuel > bl.LimitBlock {
		if mode == letGenBlock {
			return errors.WithMessage(ErrLimitStop, "txMaxFuel")
		}
		return limitError(`txMaxFuel`, `Max fuel of the block`)
	}
	return nil
}
