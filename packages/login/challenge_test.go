/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package login

import (
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var digits = regexp.MustCompile(`^\d{39}$`)

func TestUIDsAreUniqueDigits(t *testing.T) {
	c := newChallenges()
	seen := make(map[string]struct{}, 1000000)
	for range 1000000 {
		uid := c.newUID()
		require.Regexp(t, digits, uid)
		_, dup := seen[uid]
		require.False(t, dup, uid)
		seen[uid] = struct{}{}
	}
}

func TestUIDPadsSmallNumbers(t *testing.T) {
	c := newChallenges()
	c.random = func(b []byte) { clear(b); b[len(b)-1] = 7 }
	assert.Equal(t, "000000000000000000000000000000000000007", c.newUID())
	c.random = func(b []byte) {
		for i := range b {
			b[i] = 0xff
		}
	}
	assert.Equal(t, "340282366920938463463374607431768211455", c.newUID())
}

func TestChallengeIsUsedOnce(t *testing.T) {
	c := newChallenges()
	now := time.Now()
	uid, err := c.issue(now)
	require.NoError(t, err)
	assert.True(t, c.consume(uid, now))
	assert.False(t, c.consume(uid, now))
	assert.False(t, c.consume("1", now))
}

func TestChallengeIsUsedOnceConcurrently(t *testing.T) {
	c := newChallenges()
	now := time.Now()
	uid, err := c.issue(now)
	require.NoError(t, err)

	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if c.consume(uid, now) {
				wins.Add(1)
			}
		})
	}
	wg.Wait()
	assert.Equal(t, int32(1), wins.Load())
}

func TestChallengeExpires(t *testing.T) {
	c := newChallenges()
	now := time.Now()
	uid, err := c.issue(now)
	require.NoError(t, err)
	assert.False(t, c.consume(uid, now.Add(ChallengeLifetime)))

	uid, err = c.issue(now)
	require.NoError(t, err)
	assert.True(t, c.consume(uid, now.Add(ChallengeLifetime-time.Millisecond)))
}

func TestChallengesAreLimited(t *testing.T) {
	c := newChallenges()
	now := time.Now()
	for range MaxChallenges {
		_, err := c.issue(now)
		require.NoError(t, err)
	}
	_, err := c.issue(now)
	assert.Equal(t, ErrBusy, err)

	// Expired challenges make room again
	later := now.Add(ChallengeLifetime)
	_, err = c.issue(later)
	assert.NoError(t, err)
	assert.Len(t, c.open, 1)
}

func TestChallengesAreSwept(t *testing.T) {
	c := newChallenges()
	now := time.Now()
	for range 10 {
		_, err := c.issue(now)
		require.NoError(t, err)
	}
	_, err := c.issue(now.Add(ChallengeLifetime))
	require.NoError(t, err)
	assert.Len(t, c.open, 1)
}

func TestChallengeToken(t *testing.T) {
	InitSecret([]byte("test secret"))
	now := time.Now()
	uid, token, err := challenge(now)
	require.NoError(t, err)

	parsed, err := Parse(token)
	require.NoError(t, err)
	claims := parsed.Claims.(*Claims)
	assert.Equal(t, uid, claims.UID)
	assert.Len(t, claims.ID, 32)
	assert.Empty(t, claims.KeyID)
	assert.Equal(t, now.Add(ChallengeLifetime).Unix(), claims.ExpiresAt.Unix())

	_, other, err := challenge(now)
	require.NoError(t, err)
	assert.NotEqual(t, token, other)
}

func TestLoginWithoutChallenge(t *testing.T) {
	_, err := Login(Request{}, nil)
	assert.Equal(t, ErrUnknownUID, err)
	_, err = Login(Request{Token: &Claims{UID: "1"}}, nil)
	assert.Equal(t, ErrUnknownUID, err)

	uid, err := pending.issue(time.Now())
	require.NoError(t, err)
	Discard(&Claims{UID: uid})
	_, err = Login(Request{Token: &Claims{UID: uid}}, nil)
	assert.Equal(t, ErrUnknownUID, err)
}
