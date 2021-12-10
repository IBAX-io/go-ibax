/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package scheduler

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	cases := map[string]string{
		"60 * * * *":              "End of range (60) above maximum (59): 60",
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
	count int
}

func (mh *mockHandler) Run(t *Task) {
	mh.count++
}

// This test required timeout 60s
// go test -timeout 60s
func TestTask(t *testing.T) {
	var taskID = "task1"
	sch := NewScheduler()

	task := &Task{ID: taskID}

	nextTime := task.Next(time.Now())
	if nextTime != zeroTime {
		t.Error("error")
	}

	task = &Task{CronSpec: "60 * * * *"}
	err := sch.AddTask(task)
	if errStr := err.Error(); errStr != "End of range (60) above maximum (59): 60" {
		t.Error(err)
	}
	err = sch.UpdateTask(task)
	if errStr := err.Error(); errStr != "End of range (60) above maximum (59): 60" {
		t.Error(err)
	}

	err = sch.UpdateTask(&Task{ID: "task2"})
	if err != nil {
		t.Error(err)
	}

	handler := &mockHandler{}
	task = &Task{ID: taskID, CronSpec: "* * * * *", Handler: handler}
	sch.UpdateTask(task)

	now := time.Now()
	time.Sleep(task.Next(now).Sub(now) + time.Second)

	if handler.count == 0 {
		t.Error("task not running")
	}
}
