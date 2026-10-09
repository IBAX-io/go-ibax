/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package login

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

const (
	// ChallengeLifetime is how long a login challenge can be signed and used
	ChallengeLifetime = 30 * time.Second
	// MaxChallenges is how many challenges may be open at once
	MaxChallenges = 100000
	// uidDigits is the length of a uid: 128 random bits in decimal. Clients sign only digits after
	// "LOGIN" and the network id, so that a node cannot make them sign anything else.
	uidDigits = 39
)

// challenges are the uids issued and not used yet, with when each expires. They live in this
// node's memory only: a challenge is used on the node that issued it.
type challenges struct {
	mu     sync.Mutex
	open   map[string]time.Time
	swept  time.Time
	random func([]byte)
}

var pending = newChallenges()

func newChallenges() *challenges {
	return &challenges{open: make(map[string]time.Time), random: func(b []byte) { rand.Read(b) }}
}

func (c *challenges) newUID() string {
	var b [16]byte
	c.random(b[:])
	return fmt.Sprintf("%0*s", uidDigits, new(big.Int).SetBytes(b[:]).String())
}

// issue opens a challenge until now + ChallengeLifetime
func (c *challenges) issue(now time.Time) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now.Sub(c.swept) >= ChallengeLifetime || len(c.open) >= MaxChallenges {
		c.sweep(now)
	}
	if len(c.open) >= MaxChallenges {
		return "", ErrBusy
	}
	uid := c.newUID()
	for _, taken := c.open[uid]; taken; _, taken = c.open[uid] {
		uid = c.newUID()
	}
	c.open[uid] = now.Add(ChallengeLifetime)
	return uid, nil
}

// consume closes the challenge uid, and says whether it was open
func (c *challenges) consume(uid string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	expires, ok := c.open[uid]
	delete(c.open, uid)
	return ok && now.Before(expires)
}

func (c *challenges) sweep(now time.Time) {
	for uid, expires := range c.open {
		if !now.Before(expires) {
			delete(c.open, uid)
		}
	}
	c.swept = now
}

// challenge makes the token of a new login challenge
func challenge(now time.Time) (uid, token string, err error) {
	if uid, err = pending.issue(now); err != nil {
		return "", "", err
	}
	var id [16]byte
	rand.Read(id[:])
	token, err = Sign(Claims{
		UID:         uid,
		EcosystemID: "1",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        hex.EncodeToString(id[:]),
			ExpiresAt: jwt.NewNumericDate(now.Add(ChallengeLifetime)),
		},
	})
	return uid, token, err
}
