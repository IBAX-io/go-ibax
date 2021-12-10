/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package notificator

import (
	"github.com/IBAX-io/go-ibax/packages/types"
)

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

func (q *Queue) Send() {
	for _, a := range q.Accounts {
		UpdateNotifications(a.Ecosystem, a.List)
	}

	for _, r := range q.Roles {
		UpdateRolesNotifications(r.Ecosystem, r.List)
	}
}

func NewQueue() types.Notifications {
	return &Queue{
		Accounts: make([]*Accounts, 0),
		Roles:    make([]*Roles, 0),
	}
}
