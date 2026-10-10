/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package scheduler

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

// rangeError is the error of a minute out of range
const rangeError = "end of range (60) above maximum (59): 60"

func TestParse(t *testing.T) {
	cases := map[string]string{
		"60 * * * *":              rangeError,
		"0-59 0-23 1-31 1-12 0-6": "",
		"*/2 */2 */2 */2 */2":     "",
		"* * * * *":               "",
	}

	for cronSpec, expectedErr := range cases {
		_, err := Parse(cronSpec)
		if err != nil {
			if errStr := err.Error(); errStr != expectedErr {
				t.Errorf("cron: %s, expected: %s, got: %s\n", cronSpec, expectedErr, errStr)
			}

			continue
		}

		if expectedErr != "" {
			t.Errorf("cron: %s, error: %s\n", cronSpec, err)
		}
	}
}

type mockHandler struct {
	count atomic.Int32
}

func (mh *mockHandler) Run(t *Task) {
	mh.count.Add(1)
}

// entry is the entry of the cron of the scheduler for the task of the ID
func entry(sch *Scheduler, id string) (cron.Entry, int) {
	entries := sch.cron.Entries()
	for _, e := range entries {
		if e.Job.(*Task).ID == id {
			return e, len(entries)
		}
	}
	return cron.Entry{}, len(entries)
}

func TestTask(t *testing.T) {
	sch := NewScheduler()

	if next := (&Task{ID: "task1"}).Next(time.Now()); next != zeroTime {
		t.Errorf("a task without a cron spec is next at %v", next)
	}

	task := &Task{CronSpec: "60 * * * *"}
	if err := sch.AddTask(task); err == nil || err.Error() != rangeError {
		t.Errorf("AddTask of a wrong cron spec: %v", err)
	}
	if err := sch.UpdateTask(task); err == nil || err.Error() != rangeError {
		t.Errorf("UpdateTask of a wrong cron spec: %v", err)
	}

	if err := sch.UpdateTask(&Task{ID: "task2"}); err != nil {
		t.Error(err)
	}

	// The cron runs the task at the times of its schedule
	handler := &mockHandler{}
	if err := sch.UpdateTask(&Task{ID: "task1", CronSpec: "* * * * *", Handler: handler}); err != nil {
		t.Fatal(err)
	}
	e, n := entry(sch, "task1")
	if n != 2 || e.Job == nil {
		t.Fatalf("the cron has %d tasks, task1 %v", n, e.Job)
	}
	now := time.Now()
	if next := e.Schedule.Next(now); next != now.Truncate(time.Minute).Add(time.Minute) {
		t.Errorf("task1 is next at %v", next)
	}
	before := handler.count.Load()
	e.Job.Run()
	if handler.count.Load() == before {
		t.Error("the task does not run its handler")
	}

	// An update of a task of the cron changes it
	if err := sch.UpdateTask(&Task{ID: "task1", CronSpec: "0 * * * *", Handler: handler}); err != nil {
		t.Fatal(err)
	}
	if e, n = entry(sch, "task1"); n != 2 || e.Job.(*Task).CronSpec != "0 * * * *" {
		t.Errorf("the cron has %d tasks, task1 %v", n, e.Job)
	}
}
