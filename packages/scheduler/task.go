/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package scheduler

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

var zeroTime time.Time

// Handler represents interface of task handler
type Handler interface {
	Run(*Task)
}

// Task represents task
type Task struct {
	ID       string
	CronSpec string

	Handler Handler

	}

	var err error
	t.schedule, err = Parse(t.CronSpec)
	return err
}

// Next returns time for next task
func (t *Task) Next(tm time.Time) time.Time {
	if len(t.CronSpec) == 0 {
		return zeroTime
	}
	return t.schedule.Next(tm)
}

// Run executes task
func (t *Task) Run() {
	t.Handler.Run(t)
}
