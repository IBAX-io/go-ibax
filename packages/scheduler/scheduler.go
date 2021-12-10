/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package scheduler

import (
	"github.com/IBAX-io/go-ibax/packages/consts"

	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
)

var scheduler *Scheduler

func init() {
	scheduler = NewScheduler()
}

// Scheduler represents wrapper over the cron library
type Scheduler struct {
	cron *cron.Cron
}

// AddTask adds task to cron
func (s *Scheduler) AddTask(t *Task) error {
	err := t.ParseCron()
	if err != nil {
		return err
	}

	s.cron.Schedule(t, t)
	log.WithFields(log.Fields{"task": t.String()}).Info("task added")

	return nil
}

// UpdateTask updates task
func (s *Scheduler) UpdateTask(t *Task) error {
	err := t.ParseCron()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ParseError, "error": err}).Error("parse cron format")
		return err
	}

	s.cron.Stop()
	defer s.cron.Start()

	entries := s.cron.Entries()
	for _, entry := range entries {
		task := entry.Schedule.(*Task)
		if task.ID == t.ID {
			*task = *t
			log.WithFields(log.Fields{"task": t.String()}).Info("task updated")
			return nil
		}

		continue
	}

	s.cron.Schedule(t, t)
	log.WithFields(log.Fields{"task": t.String()}).Info("task added")

	return nil
}

// NewScheduler creates a new scheduler
func NewScheduler() *Scheduler {
	s := &Scheduler{cron: cron.New()}
	s.cron.Start()
	return s
}

// AddTask adds task to global scheduler
func AddTask(t *Task) error {
	return scheduler.AddTask(t)
}

// UpdateTask updates task in global scheduler
func UpdateTask(t *Task) error {
	return scheduler.UpdateTask(t)
}

// Parse parses cron format
func Parse(cronSpec string) (cron.Schedule, error) {
	sch, err := cron.ParseStandard(cronSpec)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ParseError, "error": err}).Error("parse cron format")
		return nil, err
	}
	return sch, nil
}
