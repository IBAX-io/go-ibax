/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package bftmon is the state the consensus driver of a chain of the BFT consensus shows to the
// monitoring of its node (F18 design 4.9): the driver sets it, the API reads it
package bftmon

import (
	"maps"
	"slices"
	"sync"
	"time"
)

// The alerts of design 4.9, the value of the field alert of their log entries
const (
	Stall            = "bft_stall"
	ValidatorOffline = "bft_validator_offline"
	QuorumRisk       = "bft_quorum_risk"
	SetBelow4        = "bft_set_below_4"
	ValidatorStopped = "bft_validator_stopped"
	ReviewOverdue    = "bft_review_overdue"
	ClockSkew        = "bft_clock_skew"
)

// The severities of the alerts
const (
	SeverityMajor   = "major"
	SeverityWarning = "warning"
)

// The steps of the timeouts
const (
	TimeoutPropose   = "propose"
	TimeoutPrevote   = "prevote"
	TimeoutPrecommit = "precommit"
)

// EscalationHeights is the last heights the escalation rate is of: the share of them committed
// after round 0
const EscalationHeights = 1000

// Severity is the severity of an alert, the value of the field severity of its log entries
func Severity(alert string) string {
	switch alert {
	case Stall, QuorumRisk, SetBelow4:
		return SeverityMajor
	}
	return SeverityWarning
}

// Alert is how an alert stands: Active while its condition holds, Count the log entries it made
// since the node started, Last the time of the last one
type Alert struct {
	Active bool      `json:"active"`
	Count  int64     `json:"count"`
	Last   time.Time `json:"last"`
}

// State is the state of the consensus of the node. Online is the validators of the set the node
// does not count as offline (package daemons, bft_alerts.go), Margin is Online − Quorum, Offline
// the public keys of the others, all as of the last busy height the node assessed (Assessed is
// false before the first). ClockDrift is how far the clock of the node is ahead of its time
// source. Timeouts is the timeouts that took effect at the height and round of the machine, by
// step, since the node started.
type State struct {
	Height     uint64           `json:"height"`
	Validators int              `json:"validators"`
	Quorum     int              `json:"quorum"`
	Assessed   bool             `json:"assessed"`
	Online     int              `json:"online"`
	Margin     int              `json:"margin"`
	Offline    []string         `json:"offline"`
	ClockDrift int64            `json:"clock_drift_ms"`
	Alerts     map[string]Alert `json:"alerts"`
	Timeouts   map[string]int64 `json:"timeouts"`
}

var (
	mu    sync.Mutex
	state = State{Alerts: map[string]Alert{}, Timeouts: map[string]int64{}}
)

// Update changes the state under its lock
func Update(f func(s *State)) {
	mu.Lock()
	defer mu.Unlock()
	f(&state)
}

// Raise records a log entry of an alert at now and makes it active
func Raise(alert string, now time.Time) {
	Update(func(s *State) {
		a := s.Alerts[alert]
		a.Active, a.Count, a.Last = true, a.Count+1, now
		s.Alerts[alert] = a
	})
}

// Clear makes an alert inactive
func Clear(alert string) {
	Update(func(s *State) {
		if a, ok := s.Alerts[alert]; ok && a.Active {
			a.Active = false
			s.Alerts[alert] = a
		}
	})
}

// Get is a copy of the state
func Get() State {
	mu.Lock()
	defer mu.Unlock()
	s := state
	s.Offline = slices.Clone(state.Offline)
	s.Alerts = maps.Clone(state.Alerts)
	s.Timeouts = maps.Clone(state.Timeouts)
	return s
}
