/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package notificator

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/publisher"

	"github.com/stretchr/testify/assert"
)

func TestPushQueueMerges(t *testing.T) {
	publisher.InitCentrifugo(conf.CentrifugoConfig{URL: "http://127.0.0.1:1"})
	defer publisher.InitCentrifugo(conf.CentrifugoConfig{})
	p := newPushQueue()
	p.once.Do(func() {}) // no worker: the test takes what is pending itself

	for i := 0; i < 3; i++ {
		q := NewQueue().(*Queue)
		q.AddAccounts(1, "a", "b")
		q.AddAccounts(2, "a")
		q.AddRoles(1, 7)
		p.add(q)
	}
	accounts, roles := p.take()
	assert.Equal(t, map[int64]map[string]struct{}{1: {"a": {}, "b": {}}, 2: {"a": {}}}, accounts)
	assert.Equal(t, map[int64]map[int64]struct{}{1: {7: {}}}, roles)
	accounts, roles = p.take()
	assert.Empty(t, accounts)
	assert.Empty(t, roles)
}

func TestPushQueueWithoutCentrifugo(t *testing.T) {
	p := newPushQueue()
	q := NewQueue().(*Queue)
	q.AddAccounts(1, "a")
	p.add(q)
	accounts, _ := p.take()
	assert.Empty(t, accounts)
}
