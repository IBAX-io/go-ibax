/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/service/bftmon"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

// alertDriver is a driver with no database and no network whose log entries the hook keeps
func alertDriver(t *testing.T) (*bftDriver, *test.Hook) {
	t.Helper()
	logger, hook := test.NewNullLogger()
	logger.SetLevel(log.DebugLevel)
	return &bftDriver{logger: logger.WithFields(log.Fields{})}, hook
}

// alerts is the log entries of an alert, which the hook took since it was reset
func alerts(hook *test.Hook, name string) []*log.Entry {
	var es []*log.Entry
	for _, e := range hook.AllEntries() {
		if e.Data["alert"] == name {
			es = append(es, e)
		}
	}
	return es
}

func alertSet(n int) bft.Validators {
	var set bft.Validators
	for i := range n {
		set.Keys = append(set.Keys, fmt.Appendf(nil, "validator %d", i))
	}
	return set
}

// A set below 4 is a major alert at each height the driver moves to, until the set is 4 again;
// the stop of a validator is an alert of the height its review stops it from
func TestBftAlertsOfHeight(t *testing.T) {
	d, hook := alertDriver(t)
	now := time.Now()
	for h := uint64(10); h < 13; h++ {
		d.logHeight(h, alertSet(3), nil, now)
	}
	es := alerts(hook, bftmon.SetBelow4)
	if len(es) != 3 {
		t.Fatalf("%d alerts of the set below 4 for 3 heights", len(es))
	}
	for i, e := range es {
		if e.Level != log.ErrorLevel || e.Data["severity"] != bftmon.SeverityMajor || e.Data["height"] != uint64(10+i) {
			t.Fatalf("alert %d: %v %v", i, e.Level, e.Data)
		}
	}
	if !bftmon.Get().Alerts[bftmon.SetBelow4].Active {
		t.Fatal("the set below 4 is not active")
	}
	hook.Reset()
	d.logHeight(13, alertSet(4), []sqldb.BftReview{{ID: 2, EvidenceID: 5, PublicKey: []byte{1}, StopHeight: 13, Validators: 4, Quorum: 3}}, now)
	if len(alerts(hook, bftmon.SetBelow4)) != 0 || bftmon.Get().Alerts[bftmon.SetBelow4].Active {
		t.Fatal("a set of 4 is an alert of the set below 4")
	}
	es = alerts(hook, bftmon.ValidatorStopped)
	if len(es) != 1 || es[0].Level != log.WarnLevel || es[0].Data["evidence"] != int64(5) || es[0].Data["public_key"] != "01" {
		t.Fatalf("the stop: %d alerts", len(es))
	}
}

// An overdue review is an alert as the check finds it, then each bftOverdueEvery until it
// concludes; the checks between log nothing
func TestBftAlertOverdueDaily(t *testing.T) {
	d, hook := alertDriver(t)
	t0 := time.Now()
	r := []sqldb.BftReview{{ID: 1, PublicKey: []byte{2}, StopHeight: 40, Deadline: t0.Unix() - 1}}
	at := func(after time.Duration, reviews []sqldb.BftReview) int {
		hook.Reset()
		d.logOverdue(t0.Add(after), reviews)
		return len(alerts(hook, bftmon.ReviewOverdue))
	}
	if n := at(0, r); n != 1 {
		t.Fatalf("%d alerts as the review is overdue", n)
	}
	for _, after := range []time.Duration{time.Minute, time.Hour, bftOverdueEvery - time.Minute} {
		if n := at(after, r); n != 0 {
			t.Fatalf("%d alerts %v after the first", n, after)
		}
	}
	if n := at(bftOverdueEvery, r); n != 1 {
		t.Fatalf("%d alerts a day after the first", n)
	}
	r2 := append(r, sqldb.BftReview{ID: 3, PublicKey: []byte{3}, Deadline: t0.Unix()})
	if n := at(bftOverdueEvery+time.Minute, r2); n != 1 {
		t.Fatalf("%d alerts as a second review is overdue", n)
	}
	if n := at(2*bftOverdueEvery, r2); n != 1 {
		t.Fatalf("%d alerts two days after the first", n)
	}
	if n := at(2*bftOverdueEvery+time.Minute, nil); n != 0 || bftmon.Get().Alerts[bftmon.ReviewOverdue].Active {
		t.Fatal("an alert with the reviews concluded")
	}
}

// The stall is an alert each window a busy height has no commit; a validator with no precommit
// within the window is offline once the height has been busy for a window, and a margin of 0 is
// the quorum risk; an idle chain is no stall and has no validator offline
func TestBftAlertsOfLiveness(t *testing.T) {
	d, hook := alertDriver(t)
	set := alertSet(4)
	window := 30 * time.Second
	t0 := time.Now()
	d.watch.moveTo(20, set, t0)
	precommits := func(at time.Duration, validators ...uint32) {
		for _, v := range validators {
			d.watch.precommit(&bft.Vote{Type: bft.TypePrecommit, Height: 20, Validator: v}, t0.Add(at))
		}
	}
	check := func(at time.Duration, busy bool) {
		hook.Reset()
		d.checkLiveness(t0.Add(at), busy, 0, func() []string { return []string{"silent"} })
	}
	count := func(name string) int { return len(alerts(hook, name)) }

	check(0, false)
	check(2*window, false)
	if len(hook.AllEntries()) != 0 {
		t.Fatal("alerts of an idle chain")
	}
	check(0, true)
	precommits(20*time.Second, 0, 1, 2)
	// a prevote and a precommit of another height do not count
	d.watch.precommit(&bft.Vote{Type: bft.TypePrevote, Height: 20, Validator: 3}, t0.Add(20*time.Second))
	d.watch.precommit(&bft.Vote{Type: bft.TypePrecommit, Height: 21, Validator: 3}, t0.Add(20*time.Second))
	check(window-time.Second, true)
	if len(hook.AllEntries()) != 0 {
		t.Fatal("alerts before a window")
	}
	check(window, true)
	if count(bftmon.Stall) != 1 || count(bftmon.ValidatorOffline) != 1 || count(bftmon.QuorumRisk) != 1 {
		t.Fatalf("a window: %d stalls, %d offline, %d risks", count(bftmon.Stall), count(bftmon.ValidatorOffline), count(bftmon.QuorumRisk))
	}
	e := alerts(hook, bftmon.ValidatorOffline)[0]
	if e.Data["public_key"] != fmt.Sprintf("%x", set.Keys[3]) || e.Data["margin"] != 0 {
		t.Fatalf("the validator offline: %v", e.Data)
	}
	if e := alerts(hook, bftmon.Stall)[0]; e.Level != log.ErrorLevel || e.Data["severity"] != bftmon.SeverityMajor {
		t.Fatalf("the stall: %v %v", e.Level, e.Data)
	}
	s := bftmon.Get()
	if !s.Assessed || s.Online != 3 || s.Margin != 0 || len(s.Offline) != 1 {
		t.Fatalf("the state %+v", s)
	}
	precommits(45*time.Second, 0, 1, 2)
	check(window+time.Second, true)
	check(2*window-time.Second, true)
	if count(bftmon.Stall)+count(bftmon.ValidatorOffline)+count(bftmon.QuorumRisk) != 0 {
		t.Fatal("alerts repeated within the window")
	}
	check(2*window, true)
	if count(bftmon.Stall) != 1 || count(bftmon.ValidatorOffline) != 0 || count(bftmon.QuorumRisk) != 0 {
		t.Fatalf("a second window: %d stalls, %d offline, %d risks", count(bftmon.Stall), count(bftmon.ValidatorOffline), count(bftmon.QuorumRisk))
	}

	// a commit ends the stall; the validator back online ends the risk
	d.watch.moveTo(21, set, t0.Add(2*window))
	if bftmon.Get().Alerts[bftmon.Stall].Active {
		t.Fatal("the stall is active at a new height")
	}
	d.watch.precommit(&bft.Vote{Type: bft.TypePrecommit, Height: 21, Validator: 3}, t0.Add(2*window))
	precommits(2*window, 0, 1, 2)
	check(2*window+time.Second, true)
	if len(alerts(hook, bftmon.Stall)) != 0 || bftmon.Get().Alerts[bftmon.QuorumRisk].Active || bftmon.Get().Alerts[bftmon.ValidatorOffline].Active {
		t.Fatal("alerts with every validator online")
	}
	if s := bftmon.Get(); s.Online != 4 || s.Margin != 1 || s.Height != 21 {
		t.Fatalf("the state %+v", s)
	}
}

// A validator whose last precommit, late or not, is of the height before is online however long
// the chain was idle; one that missed a height the node committed is offline as the next height is
// busy; a node that moved by more than one height counts every validator as at the height before
func TestBftOfflineByHeight(t *testing.T) {
	d, hook := alertDriver(t)
	set := alertSet(4)
	window := 30 * time.Second
	t0 := time.Now()
	at := func(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }
	precommit := func(height uint64, s float64, validators ...uint32) {
		for _, v := range validators {
			d.watch.precommit(&bft.Vote{Type: bft.TypePrecommit, Height: height, Validator: v}, at(s))
		}
	}
	check := func(s float64) {
		hook.Reset()
		d.checkLiveness(at(s), true, 0, func() []string { return nil })
	}
	state := func() string {
		s := bftmon.Get()
		return fmt.Sprint(s.Online, s.Margin, len(s.Offline))
	}

	d.watch.moveTo(30, set, at(0))
	precommit(30, 1, 0, 1, 2)
	d.watch.moveTo(31, set, at(1))
	precommit(30, 1.1, 3) // after the commit of 30
	check(100)
	if n := len(alerts(hook, bftmon.ValidatorOffline)); n != 0 || state() != "4 1 0" {
		t.Fatalf("after an idle window: %d offline, the state %s", n, state())
	}
	check(100 + window.Seconds())
	if n := len(alerts(hook, bftmon.ValidatorOffline)); n != 4 || state() != "0 -3 4" {
		t.Fatalf("a busy window with no precommit: %d offline, the state %s", n, state())
	}
	d.watch.moveTo(32, set, at(131))
	precommit(31, 131, 0, 1, 2) // late, after the commit of 31
	precommit(32, 131.5, 0, 1, 2)
	d.watch.moveTo(33, set, at(132))
	check(200)
	if s := bftmon.Get(); state() != "3 0 1" || s.Offline[0] != fmt.Sprintf("%x", set.Keys[3]) {
		t.Fatalf("validator 3 missed height 32: the state %s", state())
	}
	d.watch.moveTo(50, set, at(300))
	check(301)
	if state() != "4 1 0" {
		t.Fatalf("after a move by more than one height: the state %s", state())
	}
}

// A timeout counts if it takes effect at the height and round of the machine
func TestBftCountTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := newSignDriver(t, ctx, false)
	d.apply(d.m.Start())
	before := bftmon.Get().Timeouts
	d.countTimeout(bft.Timeout{Height: 1, Round: 1, Step: bft.StepPropose})
	d.countTimeout(bft.Timeout{Height: 2, Round: 0, Step: bft.StepPropose})
	d.countTimeout(bft.Timeout{Height: 1, Round: 0, Step: bft.StepPrevote})
	d.countTimeout(bft.Timeout{Height: 1, Round: 0, Step: bft.StepPropose})
	d.apply(d.m.OnTimeout(bft.Timeout{Height: 1, Round: 0, Step: bft.StepPropose}))
	d.countTimeout(bft.Timeout{Height: 1, Round: 0, Step: bft.StepPrecommit})
	after := bftmon.Get().Timeouts
	if after[bftmon.TimeoutPropose]-before[bftmon.TimeoutPropose] != 1 || after[bftmon.TimeoutPrevote] != before[bftmon.TimeoutPrevote] ||
		after[bftmon.TimeoutPrecommit]-before[bftmon.TimeoutPrecommit] != 1 {
		t.Fatalf("timeouts %v, before %v", after, before)
	}
}
