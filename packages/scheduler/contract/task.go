/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package contract

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/scheduler"

	log "github.com/sirupsen/logrus"
)

// ContractHandler represents contract handler
type ContractHandler struct {
	Contract string
}

// Run executes task
func (ch *ContractHandler) Run(t *scheduler.Task) {
	_, err := NodeContract(ch.Contract)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ContractError, "error": err, "task": t.String(), "contract": ch.Contract}).Error("run contract task")
		return
	}

	log.WithFields(log.Fields{"task": t.String(), "contract": ch.Contract}).Info("run contract task")
}
