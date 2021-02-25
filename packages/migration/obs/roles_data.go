/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package obs

import "github.com/IBAX-io/go-ibax/packages/consts"

var rolesDataSQL = `
INSERT INTO "1_roles" ("id", "default_page", "role_name", "deleted", "role_type", "creator","roles_access", "ecosystem") VALUES
	(next_id('1_roles'),'', 'Admin', '0', '3', '{}', '{}', '%[1]d'),
	(next_id('1_roles'),'', 'Developer', '0', '3', '{}', '{}', '%[1]d'),
	(next_id('1_roles'),'', 'Chain Consensus asbl', '0', '3', '{}', '{"rids": "1"}', '%[1]d'),
	(next_id('1_roles'),'', 'Candidate for validators', '0', '3', '{}', '{}', '%[1]d'),
	(next_id('1_roles'),'', 'Validator', '0', '3', '{}', '{}', '%[1]d'),
	(next_id('1_roles'),'', 'Investor with voting rights', '0', '3', '{}', '{}', '%[1]d'),
	(next_id('1_roles'),'', 'Delegate', '0', '3', '{}', '{}', '%[1]d');

`
