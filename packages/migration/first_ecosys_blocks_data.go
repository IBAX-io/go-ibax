/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

var firstEcosystemBlocksDataSQL = `INSERT INTO "1_blocks" (id, name, value, conditions, app_id, ecosystem) VALUES
		(next_id('1_blocks'), 'pager_header', '', 'ContractConditions("@1DeveloperCondition")', '1', '1');
`
