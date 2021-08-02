/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package updates

var M310 = `

UPDATE "1_system_parameters" 
	SET name = 'price_exec_get_contract_by_name'
	WHERE name = 'price_exec_contract_by_name';

UPDATE "1_system_parameters" 
	SET name = 'price_exec_get_contract_by_id'
	WHERE name = 'price_exec_contract_by_id';

INSERT INTO "1_system_parameters" (id, name, value, conditions) VALUES
	(next_id('1_system_parameters'), 'price_exec_send_external_transaction', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_get_block', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_int', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_get_map_keys', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_pub_to_hex', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_sqrt', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_json_encode_indent', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_encode_base64', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_sorted_keys', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_hex_to_pub', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_throw', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_create_contract', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_edit_language', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_del_table', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_string_to_bytes', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_date_time_location', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_h_mac', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_hex_to_bytes', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_split', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_get_column_type', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_unix_date_time_location', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_contract_conditions', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_random', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_get_type', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_del_column', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_update_nodes_ban', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_log10', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_validate_edit_contract_new_value', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_format_money', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_create_language', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_role_access', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_decode_base64', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_unix_date_time', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_get_history', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_floor', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_json_decode', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_update_contract', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_log', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_json_encode', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_to_lower', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_unbnd_wallet', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_get_history_row', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_block_time', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_contract_access', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_transaction_info', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_pow', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_hash', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_check_condition', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_str', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_trim_space', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_compile_contract', 'ContractAccess("@1NewContract", "@1EditContract", "@1Import")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_update_contract', 'ContractAccess("@1EditContract", "@1Import")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_create_contract', 'ContractAccess("@1NewContract", "@1Import")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_create_table', 'ContractAccess("@1NewTable", "@1NewTableJoint", "@1Import")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_flush_contract', 'ContractAccess("@1NewContract", "@1EditContract", "@1Import")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_perm_table', 'ContractAccess("@1EditTable")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_table_conditions', 'ContractAccess("@1NewTable", "@1Import", "@1NewTableJoint", "@1EditTable")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_column_condition', 'ContractAccess("@1NewColumn", "@1EditColumn")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_create_column', 'ContractAccess("@1NewColumn")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_perm_column', 'ContractAccess("@1EditColumn")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_create_language', 'ContractAccess("@1NewLang", "@1NewLangJoint", "@1Import")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_edit_language', 'ContractAccess("@1EditLang", "@1EditLangJoint", "@1Import")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_create_ecosystem', 'ContractAccess("@1NewEcosystem")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_edit_ecosys_name', 'ContractAccess("@1EditEcosystemName")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_bind_wallet', 'ContractAccess("@1BindWallet")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_unbind_wallet', 'ContractAccess("@1UnbindWallet")', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'access_exec_set_contract_wallet', 'ContractAccess("@1BindWallet", "@1UnbindWallet")', 'ContractAccess("@1UpdateSysParam")');

INSERT INTO "1_system_parameters" (id, name, value, conditions) VALUES
	(next_id('1_system_parameters'), 'price_exec_money_div', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_update_reward', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_check_sign', '50', 'ContractAccess("@1UpdateSysParam")'),
	(next_id('1_system_parameters'), 'price_exec_date_format', '50', 'ContractAccess("@1UpdateSysParam")'),
    (next_id('1_system_parameters'), 'access_exec_update_reward', 'ContractAccess("@1CallDelayedContract")', 'ContractAccess("@1UpdateSysParam")'),
    (next_id('1_system_parameters'), 'access_exec_create_view', 'ContractAccess("@1NewView")', 'ContractAccess("@1UpdateSysParam")');

`
