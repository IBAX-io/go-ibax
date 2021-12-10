/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package types

type Notifications interface {
	AddAccounts(ecosystem int64, accounts ...string)
	AddRoles(ecosystem int64, roles ...int64)
	Size() int
	Send()
}
