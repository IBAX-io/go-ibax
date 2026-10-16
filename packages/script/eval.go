/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

import (
	"sync"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	log "github.com/sirupsen/logrus"
)

type evalCode struct {
	Source string
	Code   *CodeBlock
}

// The compiled conditions by the checksum of their source. Blocks play them, and so do API requests
// that read tables and render pages, each in a goroutine of its own
var (
	evalsMutex sync.RWMutex
	evals      = make(map[uint64]*evalCode)
)

// CompileEval compiles conditional expression
func (vm *VM) CompileEval(input string, state uint32) error {
	_, err := vm.compileCondition(input, state)
	return err
}

func (vm *VM) compileCondition(input string, state uint32) (*CodeBlock, error) {
	source := `func eval bool { return ` + input + `}`
	if input == `1` || input == `0` {
		source = `
		func eval bool { 
			if ` + input + ` == 1 {
				return true
			} else {
				return false
			}
		}`
	}

	block, err := vm.CompileBlock([]rune(source), &OwnerInfo{StateID: state})
	if err != nil {
		return nil, err
	}
	crc := crypto.CalcChecksum([]byte(input))
	evalsMutex.Lock()
	evals[crc] = &evalCode{Source: input, Code: block}
	evalsMutex.Unlock()
	return block, nil
}

// compiledEval is the compiled condition of the source, nil when there is none yet
func compiledEval(input string) *CodeBlock {
	evalsMutex.RLock()
	defer evalsMutex.RUnlock()
	if eval, ok := evals[crypto.CalcChecksum([]byte(input))]; ok && eval.Source == input {
		return eval.Code
	}
	return nil
}

// EvalIf runs the conditional expression. It compiles the source code before that if that's necessary.
func (vm *VM) EvalIf(input string, state uint32, vars map[string]any) (bool, error) {
	if len(input) == 0 {
		return true, nil
	}
	code := compiledEval(input)
	if code == nil {
		var err error
		if code, err = vm.compileCondition(input, state); err != nil {
			log.WithFields(log.Fields{"type": consts.EvalError, "error": err}).Error("compiling eval")
			return false, err
		}
	}
	ret, err := NewRunTime(vm, syspar.GetMaxCost()).Run(code.Children[0], nil, vars)
	if err == nil {
		if len(ret) == 0 {
			return false, nil
		}
		return valueToBool(ret[0]), nil
	}
	return false, err
}
