/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
	(next_id('1_parameters'),'max_sum', '1000000', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'stylesheet', 'body {
		  /* You can define your custom styles here or create custom CSS rules */
	}', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'print_stylesheet', 'body {
		  /* You can define your custom styles here or create custom CSS rules */
	}', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'max_tx_block_per_user', '1000', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'min_page_validate_count', '1', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'max_page_validate_count', '6', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}'),
	(next_id('1_parameters'),'changing_blocks', 'ContractConditions("MainCondition")', 'ContractConditions("DeveloperCondition")', '{{.Ecosystem}}');
`

var parametersDataSQLMintBalance = `
INSERT INTO "1_parameters" ("id","name", "value", "conditions", "ecosystem") VALUES
	(next_id('1_parameters'),'assign_rule', '{"1":{"start_blockid":9223372036854775807,"end_blockid":9223372036854775807,"interval_blockid":7776000,"count":3,"total_amount":"63000000000000000000"},"2":{"start_blockid":9223372036854775807,"end_blockid":9223372036854775807,"interval_blockid":2592000,"count":4,"total_amount":"105000000000000000000"},"3":{"start_blockid":7776001,"end_blockid":9223372036854775807,"interval_blockid":1,"count":0,"total_amount":"315000000000000000000"},"4":{"start_blockid":3888001,"end_blockid":19440001,"interval_blockid":648000,"count":24,"total_amount":"168000000000000000000"},"5":{"start_blockid":3888001,"end_blockid":34344001,"interval_blockid":648000,"count":48,"total_amount":"315000000000000000000"},"6":{"start_blockid":7776001,"end_blockid":9223372036854775807,"interval_blockid":1,"count":0,"total_amount":"1128750000000000000000"},"7":{"start_blockid":1,"end_blockid":9223372036854775807,"interval_blockid":1,"count":0,"total_amount":"5250000000000000000"}}
', 'ContractConditions("@1DeveloperCondition")', '{{.Ecosystem}}'),
    (next_id('1_parameters'),'mint_balance', '1128750000000000000000', 'ContractConditions("@1DeveloperCondition")', '{{.Ecosystem}}'),
    (next_id('1_parameters'),'foundation_balance', '315000000000000000000', 'ContractConditions("@1DeveloperCondition")', '{{.Ecosystem}}'),
    (next_id('1_parameters'),'private_round_balance', '63000000000000000000', 'ContractConditions("@1DeveloperCondition")', '{{.Ecosystem}}'),
    (next_id('1_parameters'),'public_round_balance', '105000000000000000000', 'ContractConditions("@1DeveloperCondition")', '{{.Ecosystem}}'),
    (next_id('1_parameters'),'research_team_balance', '315000000000000000000', 'ContractConditions("@1DeveloperCondition")', '{{.Ecosystem}}'),
    (next_id('1_parameters'),'ecosystem_partners_balance', '168000000000000000000', 'ContractConditions("@1DeveloperCondition")', '{{.Ecosystem}}');
`
