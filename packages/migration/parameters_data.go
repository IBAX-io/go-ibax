/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

var parametersDataSQL = `
INSERT INTO "1_parameters" ("id","name", "value", "conditions", "ecosystem") VALUES
	(next_id('1_parameters'),'founder_account', '{{.Wallet}}', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'new_table', 'ContractConditions("DeveloperCondition")', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'changing_tables', 'ContractConditions("DeveloperCondition")', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'changing_language', 'ContractConditions("DeveloperCondition")', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'changing_page', 'ContractConditions("DeveloperCondition")', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'changing_menu', 'ContractConditions("DeveloperCondition")', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'changing_contracts', 'ContractConditions("DeveloperCondition")', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'changing_parameters', 'ContractConditions("DeveloperCondition")', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'changing_app_params', 'ContractConditions("DeveloperCondition")', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'changing_snippets', 'ContractConditions("DeveloperCondition")', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'max_sum', '1000000', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'print_stylesheet', 'body {
		  /* You can define your custom styles here or create custom CSS rules */
	}', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'min_page_validate_count', '1', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'max_page_validate_count', '6', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}');
`
