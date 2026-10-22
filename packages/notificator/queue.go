/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package notificator

import (
	"github.com/IBAX-io/go-ibax/packages/types"
)

// Queue is the notifications of the accounts and roles a transaction changed
type Queue struct {
	Accounts []*Accounts
	Roles    []*Roles
}

type Accounts struct {
	Ecosystem int64
	List      []string
}

type Roles struct {
	Ecosystem int64
	List      []int64
}

func (q *Queue) Size() int {
	return len(q.Accounts) + len(q.Roles)
}

func (q *Queue) AddAccounts(ecosystem int64, list ...string) {
	q.Accounts = append(q.Accounts, &Accounts{
		Ecosystem: ecosystem,
		List:      list,
	})
}

func (q *Queue) AddRoles(ecosystem int64, list ...int64) {
	q.Roles = append(q.Roles, &Roles{
		Ecosystem: ecosystem,
		List:      list,
	})
}

// Send hands the notifications to the pusher, which sends them after the block that made them,
// off the lock of the blocks: it neither reads the database nor calls Centrifugo itself
func (q *Queue) Send() {
	pusher.add(q)
}

func NewQueue() types.Notifications {
	return &Queue{
		Accounts: make([]*Accounts, 0),
		Roles:    make([]*Roles, 0),
	}
}
