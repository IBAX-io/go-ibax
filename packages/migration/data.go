/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

//go:generate go run ./gen/contracts.go

var (
	migrationInitial = `
	{{headseq "migration_history"}}
		t.Column("id", "int", {"default_raw": "nextval('migration_history_id_seq')"})
		t.Column("version", "string", {"default": "", "size":255})
		t.Column("date_applied", "int", {})
	{{footer "seq" "primary"}}
`
	migrationInitialTables = `

	{{head "block_chain"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("hash", "bytea", {"default": ""})
		t.Column("rollbacks_hash", "bytea", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("ecosystem_id", "int", {"default": "0"})
		t.Column("key_id", "bigint", {"default": "0"})
		t.Column("node_position", "bigint", {"default": "0"})
		t.Column("time", "int", {"default": "0"})
		t.Column("tx", "int", {"default": "0"})
	{{footer "primary" "index(node_position, time)"}}

	{{head "confirmations"}}
		t.Column("block_id", "bigint", {"default": "0"})
		t.Column("good", "int", {"default": "0"})
		t.Column("bad", "int", {"default": "0"})
		t.Column("time", "int", {"default": "0"})
	{{footer "primary(block_id)"}}

	{{head "external_blockchain"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("value", "text", {"default": ""})
		t.Column("url", "string", {"default":"", "size":255})
		t.Column("external_contract", "string", {"default":"", "size":255})
		t.Column("result_contract", "string", {"default":"", "size":255})
		t.Column("uid", "string", {"default":"", "size":255})
		t.Column("tx_time", "int", {"default":"0"})
		t.Column("sent", "int", {"default":"0"})
		t.Column("hash", "bytea", {"default":""})
		t.Column("attempts", "int", {"default":"0"})
	{{footer "primary"}}

	{{head "info_block"}}
		t.Column("hash", "bytea", {"default": ""})
		t.Column("rollbacks_hash", "bytea", {"default": ""})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("node_position", "int", {"default": "0"})
		t.Column("ecosystem_id", "bigint", {"default": "0"})
		t.Column("key_id", "bigint", {"default": "0"})
		t.Column("time", "int", {"default": "0"})
		t.Column("current_version", "string", {"default": "0.0.1", "size": 50})
		t.Column("sent", "smallint", {"default": "0"})
	{{footer "index(sent)"}}

	{{head "install"}}
		t.Column("progress", "string", {"default": "", "size":10})
	{{footer}}

	{{head "log_transactions"}}
		t.Column("hash", "bytea", {"default": ""})
		t.Column("block", "int", {"default": "0"})
	{{footer "primary(hash)"}}

	sql("DROP TYPE IF EXISTS \"my_node_keys_enum_status\" CASCADE;")
	sql("CREATE TYPE \"my_node_keys_enum_status\" AS ENUM ('my_pending','approved');")

	{{headseq "my_node_keys"}}
		t.Column("id", "bigint", {"default_raw": "nextval('my_node_keys_id_seq')"})
		t.Column("add_time", "int", {"default": "0"})
		t.Column("public_key", "bytea", {"default": ""})
		t.Column("private_key", "string", {"default": "", "size":3096})
		t.Column("status", "my_node_keys_enum_status", {"default": "my_pending"})
		t.Column("my_time", "int", {"default": "0"})
		t.Column("time", "bigint", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{head "queue_blocks"}}
		t.Column("hash", "bytea", {"default": ""})
		t.Column("honor_node_id", "bigint", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
	{{footer "primary(hash)"}}

	{{head "queue_tx"}}
		t.Column("hash", "bytea", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("from_gate", "int", {"default": "0"})
		t.Column("expedite", "decimal(30)", {"default_raw": "'0' CHECK (expedite >= 0)"})
		t.Column("time", "int", {"default": "0"})
	{{footer "primary(hash)"}}

	{{headseq "rollback_tx"}}
		t.Column("id", "bigint", {"default_raw": "nextval('rollback_tx_id_seq')"})
		t.Column("block_id", "bigint", {"default": "0"})
		t.Column("tx_hash", "bytea", {"default": ""})
		t.Column("table_name", "string", {"default": "", "size":255})
		t.Column("table_id", "string", {"default": "", "size":255})
		t.Column("data", "text", {"default": ""})
		t.Column("used", "smallint", {"default": "0"})
		t.Column("high_rate", "smallint", {"default": "0"})
		t.Column("expedite", "decimal(30)", {"default_raw": "'0' CHECK (expedite >= 0)"})
		t.Column("time", "int", {"default": "0"})
		t.Column("type", "smallint", {"default": "0"})
		t.Column("key_id", "bigint", {"default": "0"})
		t.Column("sent", "smallint", {"default": "0"})
		t.Column("verified", "smallint", {"default": "1"})
	{{footer "primary(hash)" "index(sent, used, verified, high_rate)"}}

	{{head "transactions_attempts"}}
		t.Column("hash", "bytea", {"default": ""})
		t.Column("attempt", "smallint", {"default": "0"})
	{{footer "primary(hash)" "index(attempt)"}}

	{{head "transactions_status"}}
		t.Column("hash", "bytea", {"default": ""})
		t.Column("time", "int", {"default": "0"})
		t.Column("type", "int", {"default": "0"})
		t.Column("ecosystem", "int", {"default": "1"})
		t.Column("wallet_id", "bigint", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("error", "string", {"default": "", "size":255})
		t.Column("penalty", "int", {"default": "0"})
	{{footer "primary(hash)"}}

`

	migrationInitialTablesSub = `
	{{headseq "subnode_private_packets"}}
		t.Column("id", "int", {"default_raw": "nextval('subnode_private_packets_id_seq')"})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "subnode_privatefile_packets"}}
		t.Column("id", "int", {"default_raw": "nextval('subnode_privatefile_packets_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("task_name", "text", {"default": ""})
		t.Column("task_sender", "text", {"default": ""})
		t.Column("task_type", "text", {"default": "0"})
		t.Column("mimetype", "text", {"default": ""})
		t.Column("name", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
	{{footer "seq" "primary"}}

	{{headseq "subnode_share_data_status"}}
		t.Column("id", "int", {"default_raw": "nextval('subnode_share_data_status_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("task_name", "text", {"default": ""})
		t.Column("task_sender", "text", {"default": ""})
		t.Column("task_type", "text", {"default": "0"})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("dist", "jsonb", {"null": true})
		t.Column("ecosystem", "int", {"default": "1"})
		t.Column("tcp_send_state", "int", {"default": "0"})
		t.Column("tcp_send_state_flag", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "subnode_data_uptochain_status"}}
		t.Column("id", "int", {"default_raw": "nextval('subnode_data_uptochain_status_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("time", "int", {"default": "0"})
	{{footer "seq" "primary"}}


	{{headseq "subnode_src_task"}}
		t.Column("id", "int", {"default_raw": "nextval('subnode_src_task_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("task_name", "text", {"default": ""})
		t.Column("task_sender", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("parms", "jsonb", {"null": true})
		t.Column("task_type", "int", {"default": "0"})
		t.Column("task_state", "int", {"default": "0"})
		t.Column("task_run_parms", "jsonb", {"null": true})
		t.Column("task_run_state", "int", {"default": "0"})
		t.Column("task_run_state_err", "text", {"default": ""})
		t.Column("channel_state", "int", {"default": "0"})
		t.Column("channel_state_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}
	
	{{headseq "subnode_src_data"}}
		t.Column("id", "int", {"default_raw": "nextval('subnode_src_data_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("data_info", "jsonb", {"null": true})
		t.Column("data_state", "int", {"default": "0"})
		t.Column("data_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}
	
	{{headseq "subnode_src_data_status"}}
		t.Column("id", "int", {"default_raw": "nextval('subnode_src_data_status_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("data_info", "jsonb", {"null": true})
		t.Column("tran_mode", "int", {"default": "0"})
		t.Column("subnode_src_pubkey", "text", {"default": ""})
		t.Column("subnode_dest_pubkey", "text", {"default": ""})
		t.Column("subnode_dest_ip", "text", {"default": ""})
		t.Column("subnode_agent_pubkey", "text", {"default": ""})
		t.Column("subnode_agent_ip", "text", {"default": ""})
		t.Column("agent_mode", "int", {"default": "0"})
		t.Column("data_send_state", "int", {"default": "0"})
		t.Column("data_send_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}
	
	{{headseq "subnode_src_data_chain_status"}}
		t.Column("id", "int", {"default_raw": "nextval('subnode_src_data_chain_status_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("data_info", "jsonb", {"null": true})
		t.Column("tran_mode", "int", {"default": "0"})
		t.Column("subnode_dest_pubkey", "text", {"default": ""})
		t.Column("blockchain_table", "text", {"default": ""})
		t.Column("blockchain_http", "text", {"default": ""})
		t.Column("blockchain_ecosystem", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}
	
	{{headseq "subnode_dest_data"}}
		t.Column("id", "int", {"default_raw": "nextval('subnode_dest_data_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("data_info", "jsonb", {"null": true})
		t.Column("tran_mode", "int", {"default": "0"})
		t.Column("subnode_src_pubkey", "text", {"default": ""})
		t.Column("subnode_dest_pubkey", "text", {"default": ""})
		t.Column("subnode_dest_ip", "text", {"default": ""})
		t.Column("subnode_agent_pubkey", "text", {"default": ""})
		t.Column("subnode_agent_ip", "text", {"default": ""})
		t.Column("agent_mode", "int", {"default": "0"})
		t.Column("data_state", "int", {"default": "0"})
		t.Column("data_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}
	
	{{headseq "subnode_dest_data_hash"}}
		t.Column("id", "int", {"default_raw": "nextval('subnode_dest_data_hash_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("data_info", "jsonb", {"null": true})
		t.Column("tran_mode", "int", {"default": "0"})
		t.Column("subnode_src_pubkey", "text", {"default": ""})
		t.Column("subnode_dest_pubkey", "text", {"default": ""})
		t.Column("subnode_dest_ip", "text", {"default": ""})
		t.Column("subnode_agent_pubkey", "text", {"default": ""})
		t.Column("subnode_agent_ip", "text", {"default": ""})
		t.Column("agent_mode", "int", {"default": "0"})
		t.Column("auth_state", "int", {"default": "0"})
		t.Column("sign_state", "int", {"default": "0"})
		t.Column("hash_state", "int", {"default": "0"})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}
	
	{{headseq "subnode_dest_data_status"}}
		t.Column("id", "int", {"default_raw": "nextval('subnode_dest_data_status_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("data_info", "jsonb", {"null": true})
		t.Column("tran_mode", "int", {"default": "0"})
		t.Column("subnode_src_pubkey", "text", {"default": ""})
		t.Column("subnode_dest_pubkey", "text", {"default": ""})
		t.Column("subnode_dest_ip", "text", {"default": ""})
		t.Column("subnode_agent_pubkey", "text", {"default": ""})
		t.Column("subnode_agent_ip", "text", {"default": ""})
		t.Column("agent_mode", "int", {"default": "0"})
		t.Column("auth_state", "int", {"default": "0"})
		t.Column("sign_state", "int", {"default": "0"})
		t.Column("hash_state", "int", {"default": "0"})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}
	
	{{headseq "subnode_agent_data"}}
		t.Column("id", "int", {"default_raw": "nextval('subnode_agent_data_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("data_info", "jsonb", {"null": true})
		t.Column("tran_mode", "int", {"default": "0"})
		t.Column("subnode_src_pubkey", "text", {"default": ""})
		t.Column("subnode_dest_pubkey", "text", {"default": ""})
		t.Column("subnode_dest_ip", "text", {"default": ""})
		t.Column("subnode_agent_pubkey", "text", {"default": ""})
		t.Column("subnode_agent_ip", "text", {"default": ""})
		t.Column("agent_mode", "int", {"default": "0"})
		t.Column("data_send_state", "int", {"default": "0"})
		t.Column("data_send_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}
`

	migrationInitialTablesCLB = `
	{{headseq "vde_src_task"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_src_task_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("task_name", "text", {"default": ""})
		t.Column("task_sender", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("parms", "jsonb", {"null": true})
		t.Column("task_type", "int", {"default": "0"})
		t.Column("task_state", "int", {"default": "0"})
		t.Column("contract_src_name", "text", {"default": ""})
		t.Column("contract_src_get", "text", {"default": ""})
		t.Column("contract_src_get_hash", "text", {"default": ""})
		t.Column("contract_dest_name", "text", {"default": ""})
		t.Column("contract_dest_get", "text", {"default": ""})
		t.Column("contract_dest_get_hash", "text", {"default": ""})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("contract_run_parms", "jsonb", {"null": true})
		t.Column("contract_mode", "int", {"default": "0"})
		t.Column("contract_state_src", "int", {"default": "0"})
		t.Column("contract_state_dest", "int", {"default": "0"})
		t.Column("contract_state_src_err", "text", {"default": ""})
		t.Column("contract_state_dest_err", "text", {"default": ""})
		t.Column("task_run_state", "int", {"default": "0"})
		t.Column("task_run_state_err", "text", {"default": ""})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_src_task_chain_status"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_src_task_chain_status_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("task_name", "text", {"default": ""})
		t.Column("task_sender", "text", {"default": ""})
		t.Column("task_receiver", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("parms", "jsonb", {"null": true})
		t.Column("task_type", "int", {"default": "0"})
		t.Column("task_state", "int", {"default": "0"})
		t.Column("contract_src_name", "text", {"default": ""})
		t.Column("contract_src_get", "text", {"default": ""})
		t.Column("contract_src_get_hash", "text", {"default": ""})
		t.Column("contract_dest_name", "text", {"default": ""})
		t.Column("contract_dest_get", "text", {"default": ""})
		t.Column("contract_dest_get_hash", "text", {"default": ""})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("contract_run_parms", "jsonb", {"null": true})
		t.Column("contract_mode", "int", {"default": "0"})
		t.Column("contract_state_src", "int", {"default": "0"})
		t.Column("contract_state_dest", "int", {"default": "0"})
		t.Column("contract_state_src_err", "text", {"default": ""})
		t.Column("contract_state_dest_err", "text", {"default": ""})
		t.Column("task_run_state", "int", {"default": "0"})
		t.Column("task_run_state_err", "text", {"default": ""})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_src_task_from_sche"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_src_task_from_sche_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("task_name", "text", {"default": ""})
		t.Column("task_sender", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("parms", "jsonb", {"null": true})
		t.Column("task_type", "int", {"default": "0"})
		t.Column("task_state", "int", {"default": "0"})
		t.Column("contract_src_name", "text", {"default": ""})
		t.Column("contract_src_get", "text", {"default": ""})
		t.Column("contract_src_get_hash", "text", {"default": ""})
		t.Column("contract_dest_name", "text", {"default": ""})
		t.Column("contract_dest_get", "text", {"default": ""})
		t.Column("contract_dest_get_hash", "text", {"default": ""})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("contract_run_parms", "jsonb", {"null": true})
		t.Column("contract_mode", "int", {"default": "0"})
		t.Column("contract_state_src", "int", {"default": "0"})
		t.Column("contract_state_dest", "int", {"default": "0"})
		t.Column("contract_state_src_err", "text", {"default": ""})
		t.Column("contract_state_dest_err", "text", {"default": ""})
		t.Column("task_run_state", "int", {"default": "0"})
		t.Column("task_run_state_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_src_chain_info"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_src_chain_info_id_seq')"})
		t.Column("blockchain_http", "text", {"default": ""})
		t.Column("blockchain_ecosystem", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_src_task_time"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_src_task_time_id_seq')"})
		t.Column("src_update_time", "int", {"default": "0"})
		t.Column("sche_update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_src_data"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_src_data_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("data_info", "jsonb", {"null": true})
		t.Column("data_state", "int", {"default": "0"})
		t.Column("data_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_src_data_status"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_src_data_status_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("data_info", "jsonb", {"null": true})
		t.Column("vde_src_pubkey", "text", {"default": ""})
		t.Column("vde_dest_pubkey", "text", {"default": ""})
		t.Column("vde_dest_ip", "text", {"default": ""})
		t.Column("vde_agent_pubkey", "text", {"default": ""})
		t.Column("vde_agent_ip", "text", {"default": ""})
		t.Column("agent_mode", "int", {"default": "0"})
		t.Column("data_send_state", "int", {"default": "0"})
		t.Column("data_send_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_src_data_hash"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_src_data_hash_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("blockchain_http", "text", {"default": ""})
		t.Column("blockchain_ecosystem", "text", {"default": ""})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_src_data_log"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_src_data_log_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("log", "text", {"default": ""})
		t.Column("log_type", "int", {"default": "0"})
		t.Column("log_sender", "text", {"default": ""})
		t.Column("blockchain_http", "text", {"default": ""})
		t.Column("blockchain_ecosystem", "text", {"default": ""})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_src_member"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_src_member_id_seq')"})
		t.Column("vde_pub_key", "text", {"default": ""})
		t.Column("vde_comment", "text", {"default": ""})
		t.Column("vde_name", "text", {"default": ""})
		t.Column("vde_ip", "text", {"default": ""})
		t.Column("vde_type", "int", {"default": "0"})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}


	{{headseq "vde_src_task_status"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_src_task_status_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("contract_run_parms", "jsonb", {"null": true})
		t.Column("contract_src_name", "text", {"default": ""})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_src_task_from_sche_status"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_src_task_from_sche_status_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("contract_run_parms", "jsonb", {"null": true})
		t.Column("contract_src_name", "text", {"default": ""})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_src_task_auth"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_src_task_auth_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("vde_pub_key", "text", {"default": ""})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_dest_task_from_src"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_dest_task_from_src_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("task_name", "text", {"default": ""})
		t.Column("task_sender", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("parms", "jsonb", {"null": true})
		t.Column("task_type", "int", {"default": "0"})
		t.Column("task_state", "int", {"default": "0"})
		t.Column("contract_src_name", "text", {"default": ""})
		t.Column("contract_src_get", "text", {"default": ""})
		t.Column("contract_src_get_hash", "text", {"default": ""})
		t.Column("contract_dest_name", "text", {"default": ""})
		t.Column("contract_dest_get", "text", {"default": ""})
		t.Column("contract_dest_get_hash", "text", {"default": ""})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("contract_run_parms", "jsonb", {"null": true})
		t.Column("contract_mode", "int", {"default": "0"})
		t.Column("contract_state_src", "int", {"default": "0"})
		t.Column("contract_state_dest", "int", {"default": "0"})
		t.Column("contract_state_src_err", "text", {"default": ""})
		t.Column("contract_state_dest_err", "text", {"default": ""})
		t.Column("task_run_state", "int", {"default": "0"})
		t.Column("task_run_state_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_dest_task_from_sche"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_dest_task_from_sche_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("task_name", "text", {"default": ""})
		t.Column("task_sender", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("parms", "jsonb", {"null": true})
		t.Column("task_type", "int", {"default": "0"})
		t.Column("task_state", "int", {"default": "0"})
		t.Column("contract_src_name", "text", {"default": ""})
		t.Column("contract_src_get", "text", {"default": ""})
		t.Column("contract_src_get_hash", "text", {"default": ""})
		t.Column("contract_dest_name", "text", {"default": ""})
		t.Column("contract_dest_get", "text", {"default": ""})
		t.Column("contract_dest_get_hash", "text", {"default": ""})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("contract_run_parms", "jsonb", {"null": true})
		t.Column("contract_mode", "int", {"default": "0"})
		t.Column("contract_state_src", "int", {"default": "0"})
		t.Column("contract_state_dest", "int", {"default": "0"})
		t.Column("contract_state_src_err", "text", {"default": ""})
		t.Column("contract_state_dest_err", "text", {"default": ""})
		t.Column("task_run_state", "int", {"default": "0"})
		t.Column("task_run_state_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_dest_chain_info"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_dest_chain_info_id_seq')"})
		t.Column("blockchain_http", "text", {"default": ""})
		t.Column("blockchain_ecosystem", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_dest_task_time"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_dest_task_time_id_seq')"})
		t.Column("src_update_time", "int", {"default": "0"})
		t.Column("sche_update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_dest_data"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_dest_data_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("data_info", "jsonb", {"null": true})
		t.Column("vde_src_pubkey", "text", {"default": ""})
		t.Column("vde_dest_pubkey", "text", {"default": ""})
		t.Column("vde_dest_ip", "text", {"default": ""})
		t.Column("vde_agent_pubkey", "text", {"default": ""})
		t.Column("vde_agent_ip", "text", {"default": ""})
		t.Column("agent_mode", "int", {"default": "0"})
		t.Column("data_state", "int", {"default": "0"})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_dest_data_status"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_dest_data_status_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("data_info", "jsonb", {"null": true})
		t.Column("vde_src_pubkey", "text", {"default": ""})
		t.Column("vde_dest_pubkey", "text", {"default": ""})
		t.Column("vde_dest_ip", "text", {"default": ""})
		t.Column("vde_agent_pubkey", "text", {"default": ""})
		t.Column("vde_agent_ip", "text", {"default": ""})
		t.Column("agent_mode", "int", {"default": "0"})
		t.Column("auth_state", "int", {"default": "0"})
		t.Column("sign_state", "int", {"default": "0"})
		t.Column("hash_state", "int", {"default": "0"})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_dest_data_hash"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_dest_data_hash_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("blockchain_http", "text", {"default": ""})
		t.Column("blockchain_ecosystem", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_dest_data_log"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_dest_data_log_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("log", "text", {"default": ""})
		t.Column("log_type", "int", {"default": "0"})
		t.Column("log_sender", "text", {"default": ""})
		t.Column("blockchain_http", "text", {"default": ""})
		t.Column("blockchain_ecosystem", "text", {"default": ""})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_dest_hash_time"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_dest_hash_time_id_seq')"})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_dest_member"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_dest_member_id_seq')"})
		t.Column("vde_pub_key", "text", {"default": ""})
		t.Column("vde_comment", "text", {"default": ""})
		t.Column("vde_name", "text", {"default": ""})
		t.Column("vde_ip", "text", {"default": ""})
		t.Column("vde_type", "int", {"default": "0"})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_sche_task"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_sche_task_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("task_name", "text", {"default": ""})
		t.Column("task_sender", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("parms", "jsonb", {"null": true})
		t.Column("task_type", "int", {"default": "0"})
		t.Column("task_state", "int", {"default": "0"})
		t.Column("contract_src_name", "text", {"default": ""})
		t.Column("contract_src_get", "text", {"default": ""})
		t.Column("contract_src_get_hash", "text", {"default": ""})
		t.Column("contract_dest_name", "text", {"default": ""})
		t.Column("contract_dest_get", "text", {"default": ""})
		t.Column("contract_dest_get_hash", "text", {"default": ""})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("contract_run_parms", "jsonb", {"null": true})
		t.Column("contract_mode", "int", {"default": "0"})
		t.Column("contract_state_src", "int", {"default": "0"})
		t.Column("contract_state_dest", "int", {"default": "0"})
		t.Column("contract_state_src_err", "text", {"default": ""})
		t.Column("contract_state_dest_err", "text", {"default": ""})
		t.Column("task_run_state", "int", {"default": "0"})
		t.Column("task_run_state_err", "text", {"default": ""})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_sche_task_from_src"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_sche_task_from_src_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("task_name", "text", {"default": ""})
		t.Column("task_sender", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("parms", "jsonb", {"null": true})
		t.Column("task_type", "int", {"default": "0"})
		t.Column("task_state", "int", {"default": "0"})
		t.Column("contract_src_name", "text", {"default": ""})
		t.Column("contract_src_get", "text", {"default": ""})
		t.Column("contract_src_get_hash", "text", {"default": ""})
		t.Column("contract_dest_name", "text", {"default": ""})
		t.Column("contract_dest_get", "text", {"default": ""})
		t.Column("contract_dest_get_hash", "text", {"default": ""})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("contract_run_parms", "jsonb", {"null": true})
		t.Column("contract_mode", "int", {"default": "0"})
		t.Column("contract_state_src", "int", {"default": "0"})
		t.Column("contract_state_dest", "int", {"default": "0"})
		t.Column("contract_state_src_err", "text", {"default": ""})
		t.Column("contract_state_dest_err", "text", {"default": ""})
		t.Column("task_run_state", "int", {"default": "0"})
		t.Column("task_run_state_err", "text", {"default": ""})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_sche_task_time"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_sche_task_time_id_seq')"})
		t.Column("src_update_time", "int", {"default": "0"})
		t.Column("sche_update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_sche_task_chain_status"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_sche_task_chain_status_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("task_name", "text", {"default": ""})
		t.Column("task_sender", "text", {"default": ""})
		t.Column("task_receiver", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("parms", "jsonb", {"null": true})
		t.Column("task_type", "int", {"default": "0"})
		t.Column("task_state", "int", {"default": "0"})
		t.Column("contract_src_name", "text", {"default": ""})
		t.Column("contract_src_get", "text", {"default": ""})
		t.Column("contract_src_get_hash", "text", {"default": ""})
		t.Column("contract_dest_name", "text", {"default": ""})
		t.Column("contract_dest_get", "text", {"default": ""})
		t.Column("contract_dest_get_hash", "text", {"default": ""})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("contract_run_parms", "jsonb", {"null": true})
		t.Column("contract_mode", "int", {"default": "0"})
		t.Column("contract_state_src", "int", {"default": "0"})
		t.Column("contract_state_dest", "int", {"default": "0"})
		t.Column("contract_state_src_err", "text", {"default": ""})
		t.Column("contract_state_dest_err", "text", {"default": ""})
		t.Column("task_run_state", "int", {"default": "0"})
		t.Column("task_run_state_err", "text", {"default": ""})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_sche_chain_info"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_sche_chain_info_id_seq')"})
		t.Column("blockchain_http", "text", {"default": ""})
		t.Column("blockchain_ecosystem", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_sche_member"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_sche_member_id_seq')"})
		t.Column("vde_pub_key", "text", {"default": ""})
		t.Column("vde_comment", "text", {"default": ""})
		t.Column("vde_name", "text", {"default": ""})
		t.Column("vde_ip", "text", {"default": ""})
		t.Column("vde_type", "int", {"default": "0"})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_agent_data"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_agent_data_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("hash", "text", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("data_info", "jsonb", {"null": true})
		t.Column("vde_src_pubkey", "text", {"default": ""})
		t.Column("vde_dest_pubkey", "text", {"default": ""})
		t.Column("vde_dest_ip", "text", {"default": ""})
		t.Column("vde_agent_pubkey", "text", {"default": ""})
		t.Column("vde_agent_ip", "text", {"default": ""})
		t.Column("agent_mode", "int", {"default": "0"})
		t.Column("data_send_state", "int", {"default": "0"})
		t.Column("data_send_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_agent_data_log"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_agent_data_log_id_seq')"})
		t.Column("task_uuid", "text", {"default": ""})
		t.Column("data_uuid", "text", {"default": ""})
		t.Column("log", "text", {"default": ""})
		t.Column("log_type", "int", {"default": "0"})
		t.Column("log_sender", "text", {"default": ""})
		t.Column("blockchain_http", "text", {"default": ""})
		t.Column("blockchain_ecosystem", "text", {"default": ""})
		t.Column("tx_hash", "text", {"default": ""})
		t.Column("chain_state", "int", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("chain_id", "int", {"default": "0"})
		t.Column("chain_err", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_agent_chain_info"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_agent_chain_info_id_seq')"})
		t.Column("blockchain_http", "text", {"default": ""})
		t.Column("blockchain_ecosystem", "text", {"default": ""})
		t.Column("comment", "text", {"default": ""})
		t.Column("log_mode", "int", {"default": "0"})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

	{{headseq "vde_agent_member"}}
		t.Column("id", "int", {"default_raw": "nextval('vde_agent_member_id_seq')"})
		t.Column("vde_pub_key", "text", {"default": ""})
		t.Column("vde_comment", "text", {"default": ""})
		t.Column("vde_name", "text", {"default": ""})
		t.Column("vde_ip", "text", {"default": ""})
		t.Column("vde_type", "int", {"default": "0"})
		t.Column("contract_run_http", "text", {"default": ""})
		t.Column("contract_run_ecosystem", "text", {"default": ""})
		t.Column("update_time", "int", {"default": "0"})
		t.Column("create_time", "int", {"default": "0"})
	{{footer "seq" "primary"}}

`

	migrationInitialSchema = `
		CREATE OR REPLACE FUNCTION next_id(table_name TEXT, OUT result INT) AS
		$$
		BEGIN
			EXECUTE FORMAT('SELECT COUNT(*) + 1 FROM "%s"', table_name)
			INTO result;
			RETURN;
		END
		$$
		LANGUAGE plpgsql;`
)
