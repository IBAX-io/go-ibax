/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package syspar

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

// The platform parameters of a chain of the BFT consensus (F18 design 4.3, 4.4, 4.5, 4.6, 4.9),
// which the first block of such a chain writes with their starting values and only a vote
// changes. A node reads the starting value of a parameter it cannot read.
const (
	// BftTimeoutPropose is the timeout of the propose step of round 0, in ms
	BftTimeoutPropose = `bft_timeout_propose_ms`
	// BftTimeoutPrevote is the timeout of the prevote step of round 0, in ms
	BftTimeoutPrevote = `bft_timeout_prevote_ms`
	// BftTimeoutPrecommit is the timeout of the precommit step of round 0, in ms
	BftTimeoutPrecommit = `bft_timeout_precommit_ms`
	// BftMaxFutureRounds is the rounds above its current round of which a node takes messages
	BftMaxFutureRounds = `bft_max_future_rounds`
	// BftPeerMsgRate is the consensus messages a stream carries a second at most
	BftPeerMsgRate = `bft_peer_msg_rate`
	// BftMaxClockDrift is how far the time of a block may be from the clock of a validator, in ms
	BftMaxClockDrift = `bft_max_clock_drift_ms`
	// BftStallAlarm is how long a chain with transactions may commit no block before the alarm, in ms
	BftStallAlarm = `bft_stall_alarm_ms`
	// BftEvidenceMaxAge is how many heights back an evidence is taken
	BftEvidenceMaxAge = `bft_evidence_max_age`
	// BftStopReviewDays is the days the review of a stop has to reach a conclusion
	BftStopReviewDays = `bft_stop_review_days`
	// BftReviewVotingDays is the days of the vote of a review
	BftReviewVotingDays = `bft_review_voting_days`

	// BftTimeoutDelta is what each round adds to the timeouts of round 0
	BftTimeoutDelta = 500 * time.Millisecond
	// BftProposerWait is how long a proposer waits at most for its clock to pass the time of the
	// last block, before it reads its clock and makes the block in max_block_generation_time: the
	// time of a block of an honest proposer is not ahead of its clock, so the clock of the next is
	// past it within a second but for the drift of the clocks
	BftProposerWait = time.Second
	// bftTimelinessGrowth is how much the bound behind of the timeliness grows each round, up to
	// round bftTimelinessRounds, so that a round whose messages take longer than the bound
	// assumes still takes a proposal in a later round
	bftTimelinessGrowth = 1.1
	bftTimelinessRounds = 25
)

// BftParameters is the starting values of the parameters, in the order the first block writes them
var BftParameters = [][2]string{
	{BftTimeoutPropose, "4000"},
	{BftTimeoutPrevote, "1000"},
	{BftTimeoutPrecommit, "1000"},
	{BftMaxFutureRounds, "10"},
	{BftPeerMsgRate, "500"},
	{BftMaxClockDrift, "2000"},
	{BftStallAlarm, "30000"},
	{BftEvidenceMaxAge, "100000"},
	{BftStopReviewDays, "7"},
	{BftReviewVotingDays, "3"},
}

// bftInt is a parameter of BftParameters, its starting value if it is not a positive integer
func bftInt(name string) int64 {
	if v, err := strconv.ParseInt(SysString(name), 10, 64); err == nil && v > 0 {
		return v
	}
	for _, p := range BftParameters {
		if p[0] == name {
			v, _ := strconv.ParseInt(p[1], 10, 64)
			return v
		}
	}
	return 0
}

func bftMs(name string) time.Duration {
	return time.Duration(bftInt(name)) * time.Millisecond
}

// GetBftTimeouts is the timeouts of the steps of round 0: propose, prevote and precommit
func GetBftTimeouts() (propose, prevote, precommit time.Duration) {
	return bftMs(BftTimeoutPropose), bftMs(BftTimeoutPrevote), bftMs(BftTimeoutPrecommit)
}

// GetBftMaxFutureRounds is bft_max_future_rounds
func GetBftMaxFutureRounds() uint32 {
	return uint32(min(bftInt(BftMaxFutureRounds), 1<<16))
}

// GetBftPeerMsgRate is bft_peer_msg_rate
func GetBftPeerMsgRate() int {
	return int(min(bftInt(BftPeerMsgRate), 1<<20))
}

// GetBftMaxClockDrift is bft_max_clock_drift_ms
func GetBftMaxClockDrift() time.Duration {
	return bftMs(BftMaxClockDrift)
}

// GetBftTimeliness is how far the time of the block of a new proposal of a round may be from the
// time a validator takes the proposal (design 4.4): ahead of it by at most bft_max_clock_drift_ms,
// and behind it by at most that, the second the proposer rounds its clock down to, and
// max_block_generation_time, in which the proposer makes the block after it reads its clock, so
// that only puts the time of the block behind. The bound behind grows by bftTimelinessGrowth each
// round, up to round bftTimelinessRounds.
func GetBftTimeliness(round uint32) (ahead, behind time.Duration) {
	drift := GetBftMaxClockDrift()
	behind = drift + time.Second + time.Duration(GetMaxBlockGenerationTime())*time.Millisecond
	growth := math.Pow(bftTimelinessGrowth, float64(min(round, bftTimelinessRounds)))
	return drift, time.Duration(float64(behind) * growth)
}

// bftPairs are the parameters of a chain of the BFT consensus that a vote changes only with the
// parameter they pair with, the longer one above the shorter and its margin: a proposer waits up
// to BftProposerWait for its clock and makes its block within max_block_generation_time, so a
// validator waits longer for the proposal, bft_timeout_propose_ms;
// the vote of a review lasts bft_review_voting_days, so a review has longer to reach its
// conclusion, bft_stop_review_days (design 4.5)
var bftPairs = []struct {
	longer, shorter string
	margin          int64
	reason          string
}{
	{BftTimeoutPropose, MaxBlockGenerationTime, BftProposerWait.Milliseconds(),
		"the validators would time out before the proposal of a proposer that takes its time"},
	{BftStopReviewDays, BftReviewVotingDays, 0, "a review would be overdue before the vote of its conclusion ends"},
}

// pairValue is the value a node reads of a parameter of bftPairs
func pairValue(name string) int64 {
	if name == MaxBlockGenerationTime {
		return GetMaxBlockGenerationTime()
	}
	return bftInt(name)
}

// CheckBftParameter checks a new value of a platform parameter on a chain of the BFT consensus
// against the parameter it pairs with in bftPairs
func CheckBftParameter(name, value string) error {
	for _, p := range bftPairs {
		if name != p.longer && name != p.shorter {
			continue
		}
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil || v <= 0 {
			return fmt.Errorf("%s is %q: it is a positive integer", name, value)
		}
		longer, shorter := pairValue(p.longer), pairValue(p.shorter)
		if name == p.longer {
			longer = v
		} else {
			shorter = v
		}
		if longer <= shorter+p.margin {
			return fmt.Errorf("%s %d is not above %s %d and %d: %s", p.longer, longer, p.shorter, shorter, p.margin, p.reason)
		}
	}
	return nil
}

// GetBftStallAlarm is bft_stall_alarm_ms
func GetBftStallAlarm() time.Duration {
	return bftMs(BftStallAlarm)
}

// GetBftEvidenceMaxAge is bft_evidence_max_age
func GetBftEvidenceMaxAge() uint64 {
	return uint64(bftInt(BftEvidenceMaxAge))
}

// GetBftStopReviewDays is bft_stop_review_days
func GetBftStopReviewDays() int64 {
	return bftInt(BftStopReviewDays)
}

// GetBftReviewVotingDays is bft_review_voting_days
func GetBftReviewVotingDays() int64 {
	return bftInt(BftReviewVotingDays)
}
