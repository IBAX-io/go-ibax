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
)

// NewLimits initializes Limits structure.
func NewLimits(b LimitMode) (limits *Limits) {
	limits = &Limits{Limiters: make([]Limiter, 0, 8)}

	limits.Mode = b

	allLimiters := []limiterModes{
		{limiter: &txMaxSize{}, modes: letPreprocess | letParsing},
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

// Checking the time of the start of generating block
type timeBlockLimit struct {
	Start time.Time     // the time of the start of generating block
	Limit time.Duration // the maximum time
}

func (bl *timeBlockLimit) init() {
	bl.Start = time.Now()
	bl.Limit = time.Millisecond * time.Duration(syspar.GetMaxBlockGenerationTime())
}

func (bl *timeBlockLimit) check(t TransactionCaller, mode LimitMode) error {
	if time.Since(bl.Start) < bl.Limit {
		return nil
	}

	if mode == letGenBlock {
		return errors.WithMessage(ErrLimitStop, "timeBlockLimit")
	}

	return limitError("txBlockTimeLimit", "Block generation time exceeded")
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

// Checking the max tx & block size
type txMaxSize struct {
	Size       int64 // the current size of the block
	LimitBlock int64 // max size of the block
	LimitTx    int64 // max size of tx
}

func (bl *txMaxSize) init() {
	bl.LimitBlock = syspar.GetMaxBlockSize()
	bl.LimitTx = syspar.GetMaxTxSize()
}

func (bl *txMaxSize) check(t TransactionCaller, mode LimitMode) error {
	size := int64(len(append([]byte{t.txType()}, t.txPayload()...)))
	if size > bl.LimitTx {
		return limitError(`txMaxSize`, `Max size of tx`)
	}
	bl.Size += size
	if bl.Size > bl.LimitBlock {
		if mode == letPreprocess {
			return errors.WithMessage(ErrLimitStop, "txMaxSize")
		}
		return limitError(`txMaxSize`, `Max size of the block`)
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
