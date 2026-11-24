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
	// letExecute is the execution of a block of an ordered chain (consts.BvOrdered): the order of
	// the block checked the counts and the sizes, the execution checks the fuel alone
	letExecute LimitMode = 0x0008
	// letOrder is the check of a proposal of an ordered chain: the counts and the sizes of the
	// block, which make it invalid
	letOrder LimitMode = 0x0010
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

func GetLetExecute() LimitMode {
	return letExecute
}

func GetLetOrder() LimitMode {
	return letOrder
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
	// ErrFuelLimit is a transaction past the fuel of a transaction or of its block
	ErrFuelLimit = errors.New("fuel limit")
)

// limitErr is the error of a limit: its text is that of the error of the virtual machine, and it
// is of its kind (errors.Is)
type limitErr struct {
	vm, kind error
}

func (e *limitErr) Error() string   { return e.vm.Error() }
func (e *limitErr) Unwrap() []error { return []error{e.vm, e.kind} }

// NewLimits initializes Limits structure.
func NewLimits(b LimitMode) (limits *Limits) {
	limits = &Limits{Limiters: make([]Limiter, 0, 8)}

	limits.Mode = b

	allLimiters := []limiterModes{
		{limiter: &txMaxSize{}, modes: letPreprocess | letGenBlock | letParsing | letOrder},
		{limiter: &txUserLimit{}, modes: letPreprocess | letParsing | letOrder},
		{limiter: &txMaxLimit{}, modes: letPreprocess | letParsing | letOrder},
		{limiter: &txUserEcosysLimit{}, modes: letPreprocess | letParsing | letOrder},
		{limiter: &timeBlockLimit{}, modes: letGenBlock},
		{limiter: &txMaxFuel{}, modes: letGenBlock | letParsing | letExecute},
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

// Checking the time of the start of generating block, on a chain that is not one of the BFT
// consensus: a transaction ends within the time of the block
type timeBlockLimit struct {
	Start time.Time     // the time of the start of generating block
	Limit time.Duration // the maximum time
}

func (bl *timeBlockLimit) init() {
	bl.Start = time.Now()
	bl.Limit = time.Millisecond * time.Duration(syspar.GetMaxBlockGenerationTime())
}

// left is the time left to the next transaction of the block
func (bl *timeBlockLimit) left() time.Duration {
	return max(0, bl.Limit-max(0, time.Since(bl.Start)))
}

func (bl *timeBlockLimit) check(t TransactionCaller, mode LimitMode) error {
	if bl.left() > 0 {
		return nil
	}

	if mode == letGenBlock {
		return ErrTimeLeft
	}

	return limitError("txBlockTimeLimit", "Block generation time exceeded")
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
		return &limitErr{vm: limitError(`txMaxFuel`, `Max fuel of tx %d > %d`, fuel, bl.LimitTx), kind: ErrFuelLimit}
	}
	if bl.Fuel+fuel > bl.LimitBlock {
		if mode == letGenBlock {
			return errors.WithMessage(ErrLimitStop, "txMaxFuel")
		}
		// the transaction takes no fuel of the block: the execution of an ordered block discards
		// it, and the transactions after it run on the fuel left
		return &limitErr{vm: limitError(`txMaxFuel`, `Max fuel of the block`), kind: ErrFuelLimit}
	}
	bl.Fuel += fuel
	return nil
}
