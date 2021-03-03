/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package obs

var parametersDataSQL = `
INSERT INTO "1_parameters" ("id","name", "value", "conditions", "ecosystem") VALUES
}', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'max_tx_block_per_user', '1000', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'min_page_validate_count', '1', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'max_page_validate_count', '6', 'ContractConditions("@1DeveloperCondition")', '%[1]d'),
(next_id('1_parameters'),'changing_blocks', 'ContractConditions("MainCondition")', 'ContractConditions("@1DeveloperCondition")', '%[1]d');
`
