/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// signApp proposes one block and takes every block
type signApp struct{}

func (signApp) Value(height uint64, round uint32) ([]byte, bft.Hash, bool, error) {
	b := []byte("a block")
	return b, sha256.Sum256(b), false, nil
}
func (signApp) Valid(uint64, bft.Hash, []byte) bool { return true }
func (signApp) Timely(*bft.Proposal) bool           { return true }

// signDriver is the driver of validator self of a set of 4 at height 1, with no database and no
// network: its store and transmit log what the driver stores and sends
type signDriver struct {
	*bftDriver
	failStore bool
	log       []string
	stored    *bft.SignState
	sent      []bft.Output
	// restart is a new machine of the keys of the validator, as after a restart of the node
	restart func() *bft.Machine
}

func newSignDriver(t *testing.T, ctx context.Context, proposer bool) *signDriver {
	t.Helper()
	crypto.InitAsymAlgo("ECC_P256")
	var set bft.Validators
	var keys [][2][]byte
	for range 4 {
		private, public, err := crypto.GenNodeKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, [2][]byte{private, public})
		set.Keys = append(set.Keys, public)
	}
	self := set.Proposer(1, 0)
	if !proposer {
		self = (self + 1) % 4
	}
	logger := log.WithFields(log.Fields{})
	logger.Logger.SetLevel(log.FatalLevel)
	d := &signDriver{bftDriver: &bftDriver{ctx: ctx, logger: logger, networkID: 7, events: make(chan any, bftEvents),
		bodies: newBftBodies()}}
	d.pool = bft.NewPool(7, 10, 1, set)
	d.restart = func() *bft.Machine {
		return bft.NewMachine(bft.DefaultConfig, signApp{}, keys[self][0], keys[self][1], 7, 1, set)
	}
	d.m = d.restart()
	d.store = func(s *bft.SignState) error {
		if d.failStore {
			d.log = append(d.log, "store failed")
			return errors.New("the database is down")
		}
		d.log = append(d.log, "store")
		d.stored = s
		return nil
	}
	d.transmit = func(o bft.Output) {
		d.log = append(d.log, "send")
		d.sent = append(d.sent, o)
	}
	return d
}

// has reports whether the stored state has the message the driver sent
func (d *signDriver) has(o bft.Output) bool {
	for _, p := range d.stored.Proposals {
		if o.Proposal != nil && p.Round == o.Proposal.Round && p.Hash == o.Proposal.Hash {
			return true
		}
	}
	for _, v := range d.stored.Votes {
		if o.Vote != nil && v.Round == o.Vote.Round && v.Type == o.Vote.Type && v.Hash == o.Vote.Hash {
			return true
		}
	}
	return false
}

// The driver sends a message the machine signed only after it stored the sign state that has it
// (design 4.8): a state it cannot store holds the messages, they are neither sent nor in the pool
// the driver resends from, and they go once the state is stored; a restart from the stored state
// signs nothing of a round the node signed in
func TestBftDriverStoresBeforeSending(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := newSignDriver(t, ctx, true)
	d.failStore = true
	d.apply(d.m.Start())
	if len(d.sent) != 0 || len(d.unsent) == 0 {
		t.Fatalf("with no stored state: %d messages sent, %d held", len(d.sent), len(d.unsent))
	}
	if msgs := d.pool.Messages(1); len(msgs) != 0 {
		t.Fatalf("%d messages to resend with no stored state", len(msgs))
	}
	d.failStore = false
	d.saveUnsaved()
	if len(d.sent) == 0 || d.sent[0].Proposal == nil {
		t.Fatalf("the proposal is not sent once its state is stored: %v", d.log)
	}
	if d.log[0] != "store failed" || d.log[1] != "store" {
		t.Fatalf("the order %v", d.log)
	}
	for i, o := range d.sent {
		if !d.has(o) {
			t.Fatalf("message %d sent is not in the stored state", i)
		}
	}
	if msgs := d.pool.Messages(1); len(msgs) != len(d.sent) {
		t.Fatalf("%d messages to resend, %d sent", len(msgs), len(d.sent))
	}

	// a validator that is not the proposer prevotes nil on the timeout of the proposal
	d = newSignDriver(t, ctx, false)
	d.apply(d.m.Start())
	if len(d.sent) != 0 || d.stored != nil {
		t.Fatal("a validator that is not the proposer signed on the start")
	}
	d.apply(d.m.OnTimeout(bft.Timeout{Height: 1, Round: 0, Step: bft.StepPropose}))
	if len(d.sent) != 1 || d.sent[0].Vote == nil || d.sent[0].Vote.Type != bft.TypePrevote {
		t.Fatalf("the prevote of the timeout: %d messages", len(d.sent))
	}
	if len(d.log) != 2 || d.log[0] != "store" || d.log[1] != "send" || !d.has(d.sent[0]) {
		t.Fatalf("the prevote is sent before its state is stored: %v", d.log)
	}

	// the node restarts with a new machine that restores what the node stored, and signs from
	// the round after it: nothing of round 0 again
	d.m = d.restart()
	s, err := bft.UnmarshalSignState(d.stored.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	signed := d.stored.Round
	d.sent, d.log = nil, nil
	d.apply(d.m.Restore(s))
	if d.m.Round() <= signed {
		t.Fatalf("the restart is at round %d, the node signed in round %d", d.m.Round(), signed)
	}
	d.apply(d.m.OnTimeout(bft.Timeout{Height: 1, Round: signed, Step: bft.StepPropose}))
	d.apply(d.m.OnTimeout(bft.Timeout{Height: 1, Round: d.m.Round(), Step: bft.StepPropose}))
	if len(d.sent) == 0 {
		t.Fatal("the restarted machine signs nothing in its round")
	}
	for _, o := range d.sent {
		if o.Vote != nil && o.Vote.Round <= signed || o.Proposal != nil && o.Proposal.Round <= signed {
			t.Fatalf("a message of a round the node signed in, after the restart from round %d", signed)
		}
		if !d.has(o) {
			t.Fatal("a message sent after the restart is not in the stored state")
		}
	}
}

// TestWrittenValidity checks the end of the writes of the block the node proposed tells the
// machine its validity: valid once they succeed, not valid once they fail, unknown once the node
// dropped them, and the driver hears of it as an event once they end
func TestWrittenValidity(t *testing.T) {
	logger := log.NewEntry(log.New())
	hash := bft.Hash{1}
	require.Equal(t, bft.ValidityValid, writtenValidity(logger, bftWritten{height: 1, hash: hash}))
	require.Equal(t, bft.ValidityInvalid, writtenValidity(logger, bftWritten{height: 1, hash: hash, err: errors.New("flush")}))
	require.Equal(t, bft.ValidityUnknown, writtenValidity(logger, bftWritten{height: 1, hash: hash,
		err: fmt.Errorf("writing: %w", block.ErrTrialDropped)}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &bftDriver{ctx: ctx, events: make(chan any, 1)}
	release := make(chan struct{})
	d.awaitWritten(3, hash, func() error { <-release; return errors.New("flush") })
	select {
	case <-d.events:
		t.Fatal("the event comes before the writes end")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	e := (<-d.events).(bftWritten)
	require.Equal(t, uint64(3), e.height)
	require.Equal(t, hash, e.hash)
	require.EqualError(t, e.err, "flush")
}
