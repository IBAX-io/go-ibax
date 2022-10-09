/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

import (
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	log "github.com/sirupsen/logrus"
)

type evalCode struct {
	Source string
	Code   *CodeBlock
}

var (
	evals = make(map[uint64]*evalCode)
)

// CompileEval compiles conditional expression
func (vm *VM) CompileEval(input string, state uint32) error {
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
		return err
	}
	crc := crypto.CalcChecksum([]byte(input))
	evals[crc] = &evalCode{Source: input, Code: block}
	return nil
}

// EvalIf runs the conditional expression. It compiles the source code before that if that's necessary.
func (vm *VM) EvalIf(input string, state uint32, vars map[string]any) (bool, error) {
	if len(input) == 0 {
		return true, nil
	}
	crc := crypto.CalcChecksum([]byte(input))
	if eval, ok := evals[crc]; !ok || eval.Source != input {
		if err := vm.CompileEval(input, state); err != nil {
			log.WithFields(log.Fields{"type": consts.EvalError, "error": err}).Error("compiling eval")
			return false, err
		}
	}
	ret, err := NewRunTime(vm, syspar.GetMaxCost()).Run(evals[crc].Code.Children[0], nil, vars)
	if err == nil {
		if len(ret) == 0 {
			return false, nil
		}
		return valueToBool(ret[0]), nil
	}
	return false, err
}
