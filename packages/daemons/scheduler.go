/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/scheduler"
	"github.com/IBAX-io/go-ibax/packages/scheduler/contract"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

func loadContractTasks() error {
	stateIDs, _, err := sqldb.GetAllSystemStatesIDs()
	if err != nil {
		log.WithFields(log.Fields{"error": err, "type": consts.DBError}).Error("get all system states ids")
		return err
	}

	for _, stateID := range stateIDs {
		if !sqldb.NewDbTransaction(nil).IsTable(fmt.Sprintf("%d_cron", stateID)) {
			return nil
		}

		c := sqldb.Cron{}
		c.SetTablePrefix(fmt.Sprintf("%d", stateID))
		tasks, err := c.GetAllCronTasks()
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("get all cron tasks")
			return err
		}

		for _, cronTask := range tasks {
			err = scheduler.UpdateTask(&scheduler.Task{
				ID:       cronTask.UID(),
				CronSpec: cronTask.Cron,
				Handler: &contract.ContractHandler{
					Contract: cronTask.Contract,
				},
			})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Scheduler starts contracts on schedule
func Scheduler(ctx context.Context, d *daemon) error {
	if atomic.CompareAndSwapUint32(&d.atomic, 0, 1) {
		defer atomic.StoreUint32(&d.atomic, 0)
	} else {
		return nil
	}
	d.sleepTime = time.Hour
	return loadContractTasks()
}
