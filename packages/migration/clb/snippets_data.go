/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package clb

var snippetsDataSQL = `INSERT INTO "1_snippets" (id, name, value, conditions, app_id, ecosystem) VALUES
		(next_id('1_snippets'), 'pager_header', '', 'ContractConditions("@1DeveloperCondition")', '1', '1');
`
