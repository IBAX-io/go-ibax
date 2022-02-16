/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

var pagesDataSQL = `INSERT INTO "1_pages" (id, name, value, menu, conditions, app_id, ecosystem) VALUES
	(next_id('1_pages'), 'developer_index', '', 'developer_menu', 'ContractConditions("@1DeveloperCondition")', '{{.AppID}}', '{{.Ecosystem}}');`
