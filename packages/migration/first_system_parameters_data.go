/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

var firstSystemParametersDataSQL = `
INSERT INTO "1_system_parameters" ("id","name", "value", "conditions") VALUES 
	(next_id('1_system_parameters'),'default_ecosystem_page', 'If(#ecosystem_id# > 1){Include(@1welcome)}', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'default_ecosystem_menu', '', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'default_ecosystem_contract', '', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'gap_between_blocks', '2', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'rollback_blocks', '60', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'honor_nodes', '', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'number_of_nodes', '101', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_ecosystem', '100', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_table', '1', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_column', '1', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_contract', '1', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_menu', '1', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_page', '1', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_snippet', '1', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_view', '1', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_application', '1', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_token', '5000', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_asset', '1000', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_lang', '1', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_section', '1', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'max_block_size', '67108864', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'max_tx_size', '33554432', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'max_tx_block', '1000', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'max_columns', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'max_indexes', '5', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'max_tx_block_per_user', '1000', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'max_fuel_tx', '20000000', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'max_fuel_block', '200000000', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'taxes_size', '3', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'taxes_wallet', '', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'fuel_rate', '[["1","1000000"]]', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'max_block_generation_time', '2000', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'incorrect_blocks_per_day','10','ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'node_ban_time','86400000','ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'node_ban_time_local','1800000','ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_tx_size', '15', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'price_create_rate', '1000000', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'test','false','false'),
	(next_id('1_system_parameters'),'price_tx_data', '10', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'),'private_blockchain', '1', 'false'),
	(next_id('1_system_parameters'),'pay_free_contract', '@1CallDelayedContract', 'ContractAccess("@1UpdateSysParam")'),
    (next_id('1_system_parameters'),'local_node_ban_time', '60', 'ContractAccess("@1UpdateSysParam")');
`
