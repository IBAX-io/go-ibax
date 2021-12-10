/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

import "github.com/IBAX-io/go-ibax/packages/consts"

var membersDataSQL = `
	INSERT INTO "1_members" ("id", "account", "member_name", "ecosystem") 
	VALUES
		(next_id('1_members'), '{{.Account}}', 'founder', '{{.Ecosystem}}'),
		(next_id('1_members'), '` + consts.GuestAddress + `', 'guest', '{{.Ecosystem}}');
`
