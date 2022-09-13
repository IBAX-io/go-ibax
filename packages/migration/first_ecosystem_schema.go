/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

// sqlFirstEcosystemSchema contains SQL queries for creating first ecosystem
var sqlFirstEcosystemSchema = `
	{{head "1_ecosystems"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("name", "string", {"default": "", "size":255})
		t.Column("info", "jsonb", {"null": true})
		t.Column("fee_mode_info", "jsonb", {"null": true})
		t.Column("is_valued", "bigint", {"default": "0"})
		t.Column("emission_amount", "jsonb", {"null": true})
		t.Column("token_symbol", "string", {"null": true, "size":255})
		t.Column("token_name", "string", {"null": true, "size":255})
		t.Column("type_emission", "bigint", {"default": "0"})
		t.Column("type_withdraw", "bigint", {"default": "0"})
		t.Column("control_mode", "bigint", {"default": "1"})
		t.Column("digits", "bigint", {"default": "0"})
	{{footer "primary"}}

	{{head "1_platform_parameters"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("name", "string", {"default": "", "size":255})
		t.Column("value", "text", {"default": ""})
		t.Column("conditions", "text", {"default": ""})
	{{footer "primary" "index(name)"}}

	{{head "1_delayed_contracts"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("contract", "string", {"default": "", "size":255})
		t.Column("key_id", "bigint", {"default": "0"})
		t.Column("block_id", "bigint", {"default": "0"})
		t.Column("every_block", "bigint", {"default": "0"})
		t.Column("counter", "bigint", {"default": "0"})
		t.Column("high_rate", "bigint", {"default": "0"})
		t.Column("limit", "bigint", {"default": "0"})
		t.Column("deleted", "bigint", {"default": "0"})
		t.Column("conditions", "text", {"default": ""})
	{{footer "primary" "index(block_id)"}}

	{{head "1_bad_blocks"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("producer_node_id", "bigint", {"default": "0"})
		t.Column("block_id", "bigint", {"default": "0"})
		t.Column("consumer_node_id", "bigint", {"default": "0"})
		t.Column("block_time", "timestamp", {})
		t.Column("reason", "text", {"default": ""})
		t.Column("deleted", "bigint", {"default": "0"})
	{{footer "primary" }}

	{{head "1_node_ban_logs"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("node_id", "bigint", {"default": "0"})
		t.Column("banned_at", "timestamp", {})
		t.Column("ban_time", "bigint", {"default": "0"})
		t.Column("reason", "text", {"default": ""})
	{{footer "primary" }}
`

var sqlFirstEcosystemCommon = `
	{{head "1_keys"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("pub", "bytea", {"default": ""})
		t.Column("amount", "decimal(30)", {"default_raw": "'0' CHECK (amount >= 0)"})
		t.Column("maxpay", "decimal(30)", {"default_raw": "'0' CHECK (maxpay >= 0)"})
		t.Column("multi", "bigint", {"default": "0"})
		t.Column("deleted", "bigint", {"default": "0"})
		t.Column("blocked", "bigint", {"default": "0"})
		t.Column("ecosystem", "bigint", {"default": "1"})
		t.Column("account", "char(24)", {})
		t.PrimaryKey("ecosystem", "id")
	{{footer "index(account)" "unique(ecosystem, account)"}}

	{{head "1_menu"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("name", "string", {"default": "", "size":255})
		t.Column("title", "string", {"default": "", "size":255})
		t.Column("value", "text", {"default": ""})
		t.Column("conditions", "text", {"default": ""})
		t.Column("permissions", "jsonb", {"null": true})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "unique(ecosystem, name)" "index(ecosystem, name)"}}

	{{head "1_pages"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("name", "string", {"default": "", "size":255})
		t.Column("value", "text", {"default": ""})
		t.Column("menu", "string", {"default": "", "size":255})
		t.Column("validate_count", "bigint", {"default": "1"})
		t.Column("conditions", "text", {"default": ""})
		t.Column("permissions", "jsonb", {"null": true})
		t.Column("app_id", "bigint", {"default": "1"})
		t.Column("validate_mode", "character(1)", {"default": "0"})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "unique(ecosystem, name)" "index(ecosystem, name)"}}

	{{head "1_snippets"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("name", "string", {"default": "", "size":255})
		t.Column("value", "text", {"default": ""})
		t.Column("conditions", "text", {"default": ""})
		t.Column("permissions", "jsonb", {"null": true})
		t.Column("app_id", "bigint", {"default": "1"})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "unique(ecosystem, name)" "index(ecosystem, name)"}}

	{{head "1_languages"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("name", "string", {"default": "", "size":100})
		t.Column("res", "text", {"default": ""})
		t.Column("conditions", "text", {"default": ""})
		t.Column("permissions", "jsonb", {"null": true})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "index(ecosystem, name)"}}

	{{head "1_contracts"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("name", "text", {"default": ""})
		t.Column("value", "text", {"default": ""})
		t.Column("wallet_id", "bigint", {"default": "0"})
		t.Column("token_id", "bigint", {"default": "1"})
		t.Column("conditions", "text", {"default": ""})
		t.Column("permissions", "jsonb", {"null": true})
		t.Column("app_id", "bigint", {"default": "1"})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "unique(ecosystem, name)" "index(ecosystem)"}}

	{{head "1_tables"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("name", "string", {"default": "", "size": 100})
		t.Column("permissions", "jsonb", {"null": true})
		t.Column("columns", "jsonb", {"null": true})
		t.Column("conditions", "text", {"default": ""})
		t.Column("app_id", "bigint", {"default": "1"})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "unique(ecosystem, name)" "index(ecosystem, name)"}}

	{{head "1_views"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("name", "string", {"default": "", "size": 100})
		t.Column("columns", "jsonb", {"null": true})
		t.Column("wheres", "jsonb", {"null": true})
		t.Column("permissions", "jsonb", {"null": true})
		t.Column("conditions", "text", {"default": ""})
		t.Column("app_id", "bigint", {"default": "1"})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "unique(ecosystem, name)" "index(ecosystem, name)"}}

	{{head "1_parameters"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("name", "string", {"default": "", "size": 255})
		t.Column("value", "text", {"default": ""})
		t.Column("conditions", "text", {"default": ""})
		t.Column("permissions", "jsonb", {"null": true})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "unique(ecosystem, name)" "index(ecosystem, name)"}}

	{{head "1_history"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("sender_id", "bigint", {"default": "0"})
		t.Column("recipient_id", "bigint", {"default": "0"})
		t.Column("sender_balance", "decimal(30)", {"default": "0"})
		t.Column("recipient_balance", "decimal(30)", {"default": "0"})
		t.Column("amount", "decimal(30)", {"default": "0"})
		t.Column("value_detail", "jsonb", {"null": true})
		t.Column("comment", "text", {"default": ""})
		t.Column("status", "bigint", {"default": "0"})
		t.Column("block_id", "bigint", {"default": "0"})
		t.Column("txhash", "bytea", {"default": ""})
		t.Column("created_at", "bigint", {"default": "0"})
		t.Column("ecosystem", "bigint", {"default": "1"})
		t.Column("type", "bigint", {"default": "1"})
	{{footer "primary" "index(ecosystem, sender_id)"}}
	add_index("1_history", ["ecosystem", "recipient_id"], {})
	add_index("1_history", ["block_id", "txhash"], {})

	{{head "1_sections"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("title", "string", {"default": "", "size": 255})
		t.Column("urlname", "string", {"default": "", "size": 255})
		t.Column("page", "string", {"default": "", "size": 255})
		t.Column("roles_access", "jsonb", {"null": true})
		t.Column("status", "bigint", {"default": "0"})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "index(ecosystem)"}}

	{{head "1_members"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("member_name", "string", {"default": "", "size": 255})
		t.Column("image_id", "bigint", {"default": "0"})
		t.Column("member_info", "jsonb", {"null": true})
		t.Column("ecosystem", "bigint", {"default": "1"})
		t.Column("account", "char(24)", {})
	{{footer "primary" "index(ecosystem)"}}
	add_index("1_members", ["account", "ecosystem"], {"unique": true})

	{{head "1_roles"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("default_page", "string", {"default": "", "size": 255})
		t.Column("role_name", "string", {"default": "", "size": 255})
		t.Column("deleted", "bigint", {"default": "0"})
		t.Column("role_type", "bigint", {"default": "0"})
		t.Column("creator", "jsonb", {"default": "{}"})
		t.Column("date_created", "bigint", {"default": "0"})
		t.Column("date_deleted", "bigint", {"default": "0"})
		t.Column("company_id", "bigint", {"default": "0"})
		t.Column("roles_access", "jsonb", {"null": true})
		t.Column("image_id", "bigint", {"default": "0"})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "index(ecosystem, deleted)"}}
	add_index("1_roles", ["ecosystem", "role_type"], {})

	{{head "1_roles_participants"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("role", "jsonb", {"null": true})
		t.Column("member", "jsonb", {"null": true})
		t.Column("appointed", "jsonb", {"null": true})
		t.Column("date_created", "bigint", {"default": "0"})
		t.Column("date_deleted", "bigint", {"default": "0"})
		t.Column("deleted", "bigint", {"default": "0"})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "index(ecosystem)"}}

	{{head "1_notifications"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("recipient", "jsonb", {"null": true})
		t.Column("sender", "jsonb", {"null": true})
		t.Column("notification", "jsonb", {"null": true})
		t.Column("page_params", "jsonb", {"null": true})
		t.Column("processing_info", "jsonb", {"null": true})
		t.Column("page_name", "string", {"default": "", "size": 255})
		t.Column("date_created", "bigint", {"default": "0"})
		t.Column("date_start_processing", "bigint", {"default": "0"})
		t.Column("date_closed", "bigint", {"default": "0"})
		t.Column("closed", "bigint", {"default": "0"})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "index(ecosystem)"}}

	{{head "1_applications"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("name", "string", {"default": "", "size": 255})
		t.Column("uuid", "uuid", {"default": "00000000-0000-0000-0000-000000000000"})
		t.Column("conditions", "text", {"default": ""})
		t.Column("deleted", "bigint", {"default": "0"})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "index(ecosystem)"}}

	{{head "1_binaries"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("app_id", "bigint", {"default": "1"})
		t.Column("name", "string", {"default": "", "size": 255})
		t.Column("data", "bytea", {"default": ""})
		t.Column("hash", "string", {"default": "", "size": 64})
		t.Column("mime_type", "string", {"default": "", "size": 255})
		t.Column("ecosystem", "bigint", {"default": "1"})
		t.Column("account", "char(24)", {})
	{{footer "primary"}}
	add_index("1_binaries", ["account", "ecosystem", "app_id", "name"], {"unique": true})

	{{head "1_app_params"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("app_id", "bigint", {"default": "0"})
		t.Column("name", "string", {"default": "", "size": 255})
		t.Column("value", "text", {"default": ""})
		t.Column("conditions", "text", {"default": ""})
		t.Column("permissions", "jsonb", {"null": true})
		t.Column("ecosystem", "bigint", {"default": "1"})
	{{footer "primary" "unique(ecosystem, app_id, name)" "index(ecosystem,app_id,name)"}}

	{{head "1_buffer_data"}}
		t.Column("id", "bigint", {"default": "0"})
		t.Column("key", "string", {"default": "", "size": 255})
		t.Column("value", "jsonb", {"null": true})
		t.Column("ecosystem", "bigint", {"default": "1"})
		t.Column("account", "char(24)", {})
	{{footer "primary" "index(ecosystem)"}}
`
