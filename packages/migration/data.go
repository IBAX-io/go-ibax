/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

//go:generate go run ./gen/contracts.go

var (
	migrationInitialTables = `
	{{headseq "migration_history"}}
		t.Column("id", "int", {"default_raw": "nextval('migration_history_id_seq')"})
		t.Column("version", "string", {"default": "", "size":255})
		t.Column("date_applied", "int", {})
	{{footer "seq" "primary"}}

	{{head "block_chain"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("hash", "bytea", {"default": ""})
		t.Column("rollbacks_hash", "bytea", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("ecosystem_id", "int", {"default": "0"})
		t.Column("key_id", "bigint", {"default": "0"})
		t.Column("node_position", "bigint", {"default": "0"})
		t.Column("time", "bigint", {"default": "0"})
		t.Column("tx", "int", {"default": "0"})
		t.Column("consensus_mode", "int", {"default": "1"})
		t.Column("candidate_nodes", "bytea", {"default": "\x"})
	{{footer "primary" "index(node_position, time)"}}

	{{head "confirmations"}}
		t.Column("block_id", "bigint", {"default": "0"})
		t.Column("good", "int", {"default": "0"})
		t.Column("bad", "int", {"default": "0"})
		t.Column("time", "bigint", {"default": "0"})
	{{footer "primary(block_id)"}}

	{{head "info_block"}}
		t.Column("hash", "bytea", {"default": ""})
		t.Column("rollbacks_hash", "bytea", {"default": ""})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("node_position", "int", {"default": "0"})
		t.Column("ecosystem_id", "bigint", {"default": "0"})
		t.Column("key_id", "bigint", {"default": "0"})
		t.Column("time", "bigint", {"default": "0"})
		t.Column("current_version", "string", {"default": "0.0.1", "size": 50})
		t.Column("sent", "smallint", {"default": "0"})
		t.Column("consensus_mode", "int", {"default": "1"})
		t.Column("candidate_nodes", "bytea", {"default": "\x"})
	{{footer "index(sent)"}}

	{{head "install"}}
		t.Column("progress", "string", {"default": "", "size":10})
	{{footer}}

	{{head "log_transactions"}}
		t.Column("hash", "bytea", {"default": ""})
		t.Column("block", "int", {"default": "0"})
		t.Column("timestamp", "bigint", {"default": "0"})
		t.Column("contract_name", "string", {"default": "", "size":255})
		t.Column("address", "bigint", {"default": "0"})
		t.Column("ecosystem_id", "bigint", {"default": "0"})
		t.Column("status", "bigint", {"default": "0"})
	{{footer "primary(hash)"}}

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
		t.Column("time", "bigint", {"default": "0"})
	{{footer "primary(hash)"}}

	{{headseq "rollback_tx"}}
		t.Column("id", "bigint", {"default_raw": "nextval('rollback_tx_id_seq')"})
		t.Column("block_id", "bigint", {"default": "0"})
		t.Column("tx_hash", "bytea", {"default": ""})
		t.Column("table_name", "string", {"default": "", "size":255})
		t.Column("table_id", "string", {"default": "", "size":255})
		t.Column("data_hash", "bytea", {"default": ""})
		t.Column("data", "text", {"default": ""})
	{{footer "seq" "primary" "index(table_name, table_id, block_id)"}}

	{{head "stop_daemons"}}
		t.Column("stop_time", "int", {"default": "0"})
	{{footer}}

	{{head "spent_info"}}
		t.Column("input_tx_hash", "bytea", {"null": true})
		t.Column("input_index", "integer", {"null": true})
		t.Column("output_tx_hash", "bytea")
		t.Column("output_index", "integer")
		t.Column("output_key_id", "bigint")
		t.Column("output_value", "decimal(30)", {"default_raw": "'0' CHECK (output_value >= 0)"})
		t.Column("ecosystem", "bigint", {"default": "1"})
		t.Column("block_id", "bigint")
		t.Column("type", "bigint")
	{{footer "primary(output_tx_hash,output_key_id,output_index)" "index(block_id)" "index(input_tx_hash)" "index(output_key_id)" "index(output_tx_hash)"}}

	{{head "transactions"}}
		t.Column("hash", "bytea", {"default": ""})
		t.Column("data", "bytea", {"default": ""})
		t.Column("used", "smallint", {"default": "0"})
		t.Column("high_rate", "smallint", {"default": "0"})
		t.Column("expedite", "decimal(30)", {"default_raw": "'0' CHECK (expedite >= 0)"})
		t.Column("time", "bigint", {"default": "0"})
		t.Column("type", "smallint", {"default": "0"})
		t.Column("key_id", "bigint", {"default": "0"})
		t.Column("sent", "smallint", {"default": "0"})
		t.Column("verified", "smallint", {"default": "1"})
	{{footer "primary(hash)" "index(sent, used, verified, high_rate)"}}

	{{head "transactions_status"}}
		t.Column("hash", "bytea", {"default": ""})
		t.Column("time", "bigint", {"default": "0"})
		t.Column("type", "int", {"default": "0"})
		t.Column("ecosystem", "int", {"default": "1"})
		t.Column("wallet_id", "bigint", {"default": "0"})
		t.Column("block_id", "int", {"default": "0"})
		t.Column("error", "string", {"default": "", "size":255})
		t.Column("penalty", "int", {"default": "0"})
	{{footer "primary(hash)"}}

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

	tentative = `
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

	{{head "transactions_attempts"}}
		t.Column("hash", "bytea", {"default": ""})
		t.Column("attempt", "smallint", {"default": "0"})
	{{footer "primary(hash)" "index(attempt)"}}

	`
)
