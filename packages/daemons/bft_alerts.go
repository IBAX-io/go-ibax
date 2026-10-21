/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/service/bftmon"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

// The alerts of the driver (F18 design 4.9). Each log entry of an alert has the field alert, its
// name in package bftmon, and the field severity: major for the stall, the quorum risk and a set
// below 4, which the node logs as errors, warning for the others. The window of the stall, of the
// validators offline and of the quorum risk is bft_stall_alarm_ms.
//
// The node records the last valid precommit, of any hash, of each validator at its height or at
// the height before, whose last precommits reach it after it committed that height. A validator
// is offline if no precommit of it reached the node within the window and it either missed a
// height the node committed, its last precommit is of a height below the one before, or the
// height has been busy, with a transaction to put in a block, for a window. The margin of the
// quorum is the validators online less the quorum. The node assesses them at the busy heights
// only: an idle chain has no round, and no validator misses one. A validator the node has no
// precommit of counts as precommitting the height before the one the node started at, or moved
// to by more than one height, as the node then took the blocks by their certificates.
//
// The stall repeats each window a height stays busy without a commit, the set below 4 each height
// the node moves to, the overdue review each bftOverdueEvery; the others are logged as they start.

const (
	// bftOverdueCheck is how often the driver reads the reviews past their deadline
	bftOverdueCheck = time.Minute
	// bftClockEvery is how often the driver measures its clock against the time source
	bftClockEvery = 10 * time.Minute
	// bftTimeSource is the time source of a node that sets none
	bftTimeSource = "pool.ntp.org:123"
)

// bftSeen is the last valid precommit of a validator that reached the node
type bftSeen struct {
	at     time.Time
	height uint64
}

// bftWatch is the state of the alerts of the driver. seen, base, height, set and prev are guarded
// by mu, as the handler of the service records the precommits; run uses the others.
type bftWatch struct {
	mu     sync.Mutex
	seen   map[string]bftSeen // by the hex of the public key
	base   bftSeen            // the precommit of a validator the node has none of
	height uint64
	set    bft.Validators
	prev   bft.Validators // the set of the height before, empty if the node did not run it

	workSince   time.Time // when the height got a transaction to put in a block
	stallLogged time.Time
	offline     map[string]bool
	risk        bool

	overdue        map[int64]time.Time // the overdue reviews, and when each was last logged
	overdueChecked time.Time

	clockChecked time.Time
	clockBusy    atomic.Bool
}

// alert logs an alert and records it in bftmon
func (d *bftDriver) alert(name string, now time.Time, fields log.Fields, msg string) {
	fields["alert"], fields["severity"] = name, bftmon.Severity(name)
	if _, ok := fields["type"]; !ok {
		fields["type"] = consts.BlockError
	}
	e := d.logger.WithFields(fields)
	if bftmon.Severity(name) == bftmon.SeverityMajor {
		e.Error(msg)
	} else {
		e.Warn(msg)
	}
	bftmon.Raise(name, now)
}

// precommit records a valid precommit of a validator of the current height or of the one before
func (w *bftWatch) precommit(v *bft.Vote, now time.Time) {
	if v.Type != bft.TypePrecommit {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	set := w.set
	if v.Height+1 == w.height {
		set = w.prev
	} else if v.Height != w.height {
		return
	}
	key, ok := set.Key(v.Validator)
	if !ok {
		return
	}
	if w.seen == nil {
		w.seen = map[string]bftSeen{}
	}
	k := fmt.Sprintf("%x", key)
	w.seen[k] = bftSeen{at: now, height: max(v.Height, w.seen[k].height)}
}

// moveTo starts the watch of a height and its set at now
func (w *bftWatch) moveTo(height uint64, set bft.Validators, now time.Time) {
	w.mu.Lock()
	w.prev = bft.Validators{}
	if height == w.height+1 {
		w.prev = w.set
	} else {
		w.base = bftSeen{at: now, height: height - 1}
	}
	w.height, w.set = height, set
	w.mu.Unlock()
	w.workSince, w.stallLogged = time.Time{}, time.Time{}
	bftmon.Clear(bftmon.Stall)
	bftmon.Update(func(s *bftmon.State) {
		s.Height, s.Validators, s.Quorum = height, set.N(), set.Quorum()
	})
}

// onHeight logs the alerts of the heights after from up to the height the driver moved to, of
// set: the set below 4, and the validators the evidence of these heights stops
func (d *bftDriver) onHeight(from, height uint64, set bft.Validators, now time.Time) {
	reviews, err := (&sqldb.BftReview{}).GetStoppedIn(nil, int64(from), int64(height))
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting the stopped validators")
	}
	d.logHeight(height, set, reviews, now)
}

// logHeight logs the alerts of the height the driver moved to: the set below 4, and the stops of
// the reviews
func (d *bftDriver) logHeight(height uint64, set bft.Validators, reviews []sqldb.BftReview, now time.Time) {
	if set.N() < 4 {
		d.alert(bftmon.SetBelow4, now, log.Fields{"height": height, "validators": set.N(), "faults": set.Faults()},
			"the validators set is below 4: it bears no faulty validator, the consortium adds validators")
	} else {
		bftmon.Clear(bftmon.SetBelow4)
	}
	for _, r := range reviews {
		d.alert(bftmon.ValidatorStopped, now, log.Fields{"public_key": fmt.Sprintf("%x", r.PublicKey), "evidence": r.EvidenceID,
			"review": r.ID, "stop_height": r.StopHeight, "deadline": time.Unix(r.Deadline, 0).UTC(),
			"validators": r.Validators, "quorum": r.Quorum}, "a validator is stopped by an evidence")
	}
}

// checkLiveness checks the stall, the validators offline and the quorum risk at now; busy is
// whether the height has a transaction to put in a block, silent the validators of the set with
// no vote at the height
func (d *bftDriver) checkLiveness(now time.Time, busy bool, round uint32, silent func() []string) {
	w := &d.watch
	if !busy {
		w.workSince = time.Time{}
		return
	}
	window := syspar.GetBftStallAlarm()
	if w.workSince.IsZero() {
		w.workSince = now
	}
	w.mu.Lock()
	height, set := w.height, w.set
	w.mu.Unlock()
	if now.Sub(w.workSince) >= window && now.Sub(w.stallLogged) >= window {
		w.stallLogged = now
		d.alert(bftmon.Stall, now, log.Fields{"height": height, "round": round, "since": w.workSince.UTC(), "silent": silent()},
			"no commit with transactions waiting: more than f validators are offline or unreachable, the chain goes on once they are back")
	}
	w.mu.Lock()
	var online int
	var offline []string
	last := map[string]bftSeen{}
	for _, k := range set.Keys {
		key := fmt.Sprintf("%x", k)
		s := w.seen[key]
		if s.at.Before(w.base.at) {
			s = w.base
		}
		if now.Sub(s.at) >= window && (s.height+1 < height || now.Sub(w.workSince) >= window) {
			offline = append(offline, key)
			last[key] = s
		} else {
			online++
		}
	}
	w.mu.Unlock()
	margin := online - set.Quorum()
	if w.offline == nil {
		w.offline = map[string]bool{}
	}
	for _, key := range offline {
		if !w.offline[key] {
			w.offline[key] = true
			d.alert(bftmon.ValidatorOffline, now, log.Fields{"public_key": key, "last_height": last[key].height,
				"height": height, "margin": margin}, "a validator sent no valid precommit within the window")
		}
	}
	for key := range w.offline {
		if slices.Contains(offline, key) {
			continue
		}
		delete(w.offline, key)
		if slices.ContainsFunc(set.Keys, func(k []byte) bool { return fmt.Sprintf("%x", k) == key }) {
			d.logger.WithFields(log.Fields{"public_key": key, "height": height}).Info("a validator is back online")
		}
	}
	if len(offline) == 0 {
		bftmon.Clear(bftmon.ValidatorOffline)
	}
	switch {
	case margin <= 0 && !w.risk:
		w.risk = true
		d.alert(bftmon.QuorumRisk, now, log.Fields{"height": height, "margin": margin, "online": online,
			"quorum": set.Quorum(), "offline": offline}, "one more validator offline stops the chain")
	case margin > 0 && w.risk:
		w.risk = false
		bftmon.Clear(bftmon.QuorumRisk)
	}
	bftmon.Update(func(s *bftmon.State) {
		s.Assessed, s.Online, s.Margin, s.Offline = true, online, margin, offline
	})
}

// checkOverdue logs, each bftOverdueCheck, the reviews open past their deadline (design 4.5): a
// review as it is overdue, and each bftOverdueEvery after until it concludes. The node stays out
// of the set until the vote of its review concludes.
func (d *bftDriver) checkOverdue(now time.Time) {
	w := &d.watch
	if now.Sub(w.overdueChecked) < bftOverdueCheck {
		return
	}
	w.overdueChecked = now
	reviews, err := (&sqldb.BftReview{}).GetOverdue(nil, now.Unix())
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting the overdue reviews")
		return
	}
	d.logOverdue(now, reviews)
}

// logOverdue logs the overdue reviews of a check at now
func (d *bftDriver) logOverdue(now time.Time, reviews []sqldb.BftReview) {
	w := &d.watch
	kept := map[int64]time.Time{}
	for _, r := range reviews {
		logged, ok := w.overdue[r.ID]
		if !ok || now.Sub(logged) >= bftOverdueEvery {
			logged = now
			d.alert(bftmon.ReviewOverdue, now, log.Fields{"review": r.ID, "public_key": fmt.Sprintf("%x", r.PublicKey),
				"stop_height": r.StopHeight, "deadline": time.Unix(r.Deadline, 0).UTC()}, "the review of a stopped validator is overdue")
		}
		kept[r.ID] = logged
	}
	w.overdue = kept
	if len(kept) == 0 {
		bftmon.Clear(bftmon.ReviewOverdue)
	}
}

// checkClock measures, each bftClockEvery and out of run, the drift of the clock of the node
// from its time source, LocalConf.TimeSource, and logs a drift above half of
// bft_max_clock_drift_ms (design 4.4)
func (d *bftDriver) checkClock(now time.Time) {
	w := &d.watch
	if now.Sub(w.clockChecked) < bftClockEvery || !w.clockBusy.CompareAndSwap(false, true) {
		return
	}
	w.clockChecked = now
	source := conf.Config.LocalConf.TimeSource
	if source == "" {
		source = bftTimeSource
	}
	go func() {
		defer w.clockBusy.Store(false)
		drift, err := d.clockDrift(source)
		if err != nil {
			d.logger.WithFields(log.Fields{"type": consts.NetworkError, "error": err, "source": source}).Warn("measuring the clock against the time source")
			return
		}
		bftmon.Update(func(s *bftmon.State) { s.ClockDrift = drift.Milliseconds() })
		if limit := syspar.GetBftMaxClockDrift() / 2; drift > limit || drift < -limit {
			d.alert(bftmon.ClockSkew, time.Now(), log.Fields{"drift_ms": drift.Milliseconds(), "limit_ms": limit.Milliseconds(),
				"source": source}, "the clock of the node drifts from the time source")
		} else {
			bftmon.Clear(bftmon.ClockSkew)
		}
	}()
}

// countTimeout counts a timeout that takes effect at the height and round of the machine: the
// timeout of the proposal and of the prevotes at their step, the timeout of the precommits always
func (d *bftDriver) countTimeout(t bft.Timeout) {
	if d.m.Decided() || t.Height != d.m.Height() || t.Round != d.m.Round() {
		return
	}
	var step string
	switch {
	case t.Step == bft.StepPropose && d.m.Step() == bft.StepPropose:
		step = bftmon.TimeoutPropose
	case t.Step == bft.StepPrevote && d.m.Step() == bft.StepPrevote:
		step = bftmon.TimeoutPrevote
	case t.Step == bft.StepPrecommit:
		step = bftmon.TimeoutPrecommit
	default:
		return
	}
	bftmon.Update(func(s *bftmon.State) { s.Timeouts[step]++ })
}

// silent is the validators of the set of the height with no vote there
func (d *bftDriver) silent() []string {
	d.mu.Lock()
	msgs := d.pool.Messages(d.m.Height())
	d.mu.Unlock()
	voted := map[uint32]bool{}
	for _, m := range msgs {
		if v, ok := m.(*bft.Vote); ok {
			voted[v.Validator] = true
		}
	}
	var keys []string
	d.watch.mu.Lock()
	set := d.watch.set
	d.watch.mu.Unlock()
	for i, k := range set.Keys {
		if !voted[uint32(i)] {
			keys = append(keys, fmt.Sprintf("%x", k))
		}
	}
	return keys
}
