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
