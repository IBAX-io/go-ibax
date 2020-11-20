/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package contract

import (
	"github.com/IBAX-io/go-ibax/packages/consts"

// Run executes task
func (ch *ContractHandler) Run(t *scheduler.Task) {
	_, err := NodeContract(ch.Contract)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ContractError, "error": err, "task": t.String(), "contract": ch.Contract}).Error("run contract task")
		return
	}

	log.WithFields(log.Fields{"task": t.String(), "contract": ch.Contract}).Info("run contract task")
}
