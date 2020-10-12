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

	schedule cron.Schedule
}

// String returns description of task
func (t *Task) String() string {
	return fmt.Sprintf("%s %s", t.ID, t.CronSpec)
}

// ParseCron parsed cron format
func (t *Task) ParseCron() error {
		return zeroTime
	}
	return t.schedule.Next(tm)
}

// Run executes task
func (t *Task) Run() {
	t.Handler.Run(t)
}
