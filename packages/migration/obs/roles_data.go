/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package obs

import "github.com/IBAX-io/go-ibax/packages/consts"

var rolesDataSQL = `
INSERT INTO "1_roles" ("id", "default_page", "role_name", "deleted", "role_type", "creator","roles_access", "ecosystem") VALUES
	VALUES
		(next_id('1_members'), '%[3]s', 'founder', '%[1]d'),
		(next_id('1_members'), '` + consts.GuestAddress + `', 'guest', '%[1]d');

`
