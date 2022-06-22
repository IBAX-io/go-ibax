/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package clb

var parametersDataSQL = `
INSERT INTO "1_parameters" ("id","name", "value", "conditions", "ecosystem") VALUES
(next_id('1_parameters'),'founder_account', '%[2]d', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'new_table', 'ContractConditions("MainCondition")', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'changing_tables', 'ContractConditions("MainCondition")', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'changing_language', 'ContractConditions("MainCondition")', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'changing_page', 'ContractConditions("MainCondition")', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'changing_menu', 'ContractConditions("MainCondition")', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'changing_contracts', 'ContractConditions("MainCondition")', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'changing_parameters', 'ContractConditions("MainCondition")', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'changing_app_params', 'ContractConditions("MainCondition")', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'max_sum', '1000000', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'stylesheet', 'body {
	/* You can define your custom styles here or create custom CSS rules */
}', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'print_stylesheet', 'body {
	/* You can define your custom styles here or create custom CSS rules */
}', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'min_page_validate_count', '1', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'max_page_validate_count', '6', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'changing_snippets', 'ContractConditions("MainCondition")', 'ContractConditions("@1DeveloperCondition")', '%[1]d');
`
