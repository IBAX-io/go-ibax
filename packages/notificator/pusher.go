/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package notificator

import (
	"encoding/json"
	"sync"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/publisher"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

// pusher sends the notifications of committed blocks to Centrifugo in the background. The blocks
// add the accounts and roles they changed; what is pending merges, so an account changed by many
// transactions while the pusher is busy gets one message, with its counts read after them all.
// The block path thus makes no network call and no query for notifications.
var pusher = newPushQueue()

type pushQueue struct {
	mu       sync.Mutex
	accounts map[int64]map[string]struct{}
	roles    map[int64]map[int64]struct{}
	wake     chan struct{}
	once     sync.Once
}

func newPushQueue() *pushQueue {
	return &pushQueue{
		accounts: make(map[int64]map[string]struct{}),
		roles:    make(map[int64]map[int64]struct{}),
		wake:     make(chan struct{}, 1),
	}
}

// add merges the notifications of q into what is pending and wakes the pusher. A node without
// Centrifugo keeps nothing.
func (p *pushQueue) add(q *Queue) {
	if q.Size() == 0 || !publisher.Configured() {
		return
	}
	p.once.Do(func() { go p.run() })
	p.mu.Lock()
	for _, a := range q.Accounts {
		set := p.accounts[a.Ecosystem]
		if set == nil {
			set = make(map[string]struct{}, len(a.List))
			p.accounts[a.Ecosystem] = set
		}
		for _, account := range a.List {
			set[account] = struct{}{}
		}
	}
	for _, r := range q.Roles {
		set := p.roles[r.Ecosystem]
		if set == nil {
			set = make(map[int64]struct{}, len(r.List))
			p.roles[r.Ecosystem] = set
		}
		for _, role := range r.List {
			set[role] = struct{}{}
		}
	}
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// take returns what is pending and leaves nothing
func (p *pushQueue) take() (map[int64]map[string]struct{}, map[int64]map[int64]struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	accounts, roles := p.accounts, p.roles
	p.accounts = make(map[int64]map[string]struct{})
	p.roles = make(map[int64]map[int64]struct{})
	return accounts, roles
}

func (p *pushQueue) run() {
	for range p.wake {
		accounts, roles := p.take()
		for ecosystem, set := range roles {
			list := make([]int64, 0, len(set))
			for role := range set {
				list = append(list, role)
			}
			members, err := sqldb.GetRoleMembers(nil, ecosystem, list)
			if err != nil {
				log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting role members for notifications")
				continue
			}
			if accounts[ecosystem] == nil {
				accounts[ecosystem] = make(map[string]struct{}, len(members))
			}
			for _, m := range members {
				accounts[ecosystem][m] = struct{}{}
			}
		}
		var msgs []publisher.Message
		for ecosystem, set := range accounts {
			list := make([]string, 0, len(set))
			for account := range set {
				list = append(list, account)
			}
			stats, err := getEcosystemNotificationStats(ecosystem, list)
			if err != nil {
				continue
			}
			for account, n := range stats {
				raw, err := json.Marshal(*n)
				if err != nil {
					log.WithFields(log.Fields{"type": consts.JSONMarshallError, "error": err}).Error("notification statistic")
					continue
				}
				msgs = append(msgs, publisher.Message{Account: account, Data: string(raw)})
			}
		}
		if err := publisher.WriteBatch(msgs); err != nil {
			log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Debug("writing to centrifugo")
		}
	}
}
