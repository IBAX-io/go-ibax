/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package clb

import (
	"strings"
)

// GetCLBScript returns script to create ecosystem
func GetCLBScript() string {
	scripts := []string{
		schemaCLB,
		snippetsDataSQL,
		contractsDataSQL,
		menuDataSQL,
		pagesDataSQL,
		parametersDataSQL,
		rolesDataSQL,
		sectionsDataSQL,
		tablesDataSQL,
		applicationsDataSQL,
		keysDataSQL,
		platformParametersDataSQL,
	}

	return strings.Join(scripts, "\r\n")
}

// SchemaEcosystem contains SQL queries for creating ecosystem
var schemaCLB = `DROP TABLE IF EXISTS "1_keys"; CREATE TABLE "1_keys" (
		"id" bigint  NOT NULL DEFAULT '0',
		"pub" bytea  NOT NULL DEFAULT '',
		"amount" decimal(30) NOT NULL DEFAULT '0' CHECK (amount >= 0),
		"multi" bigint NOT NULL DEFAULT '0',
		"deleted" bigint NOT NULL DEFAULT '0',
		"blocked" bigint NOT NULL DEFAULT '0',
		"ecosystem" bigint NOT NULL DEFAULT '1',
		"account" char(24) NOT NULL
		);
		ALTER TABLE ONLY "1_keys" ADD CONSTRAINT "%[1]d_keys_pkey" PRIMARY KEY (id,ecosystem);
		
		DROP TABLE IF EXISTS "1_history"; CREATE TABLE "1_history" (
		"id" bigint NOT NULL  DEFAULT '0',
		"sender_id" bigint NOT NULL DEFAULT '0',
		"recipient_id" bigint NOT NULL DEFAULT '0',
		"amount" decimal(30) NOT NULL DEFAULT '0',
		"comment" text NOT NULL DEFAULT '',
		"block_id" int  NOT NULL DEFAULT '0',
		"txhash" bytea  NOT NULL DEFAULT '',
		"created_at" bigint NOT NULL DEFAULT '0',
		"ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_history" ADD CONSTRAINT "%[1]d_history_pkey" PRIMARY KEY (id);
		CREATE INDEX "%[1]d_history_index_sender" ON "%[1]d_history" (sender_id);
		CREATE INDEX "%[1]d_history_index_recipient" ON "%[1]d_history" (recipient_id);
		CREATE INDEX "%[1]d_history_index_block" ON "%[1]d_history" (block_id, txhash);
		
		
		DROP TABLE IF EXISTS "%[1]d_languages"; CREATE TABLE "%[1]d_languages" (
		  "id" bigint  NOT NULL DEFAULT '0',
		  "name" character varying(100) NOT NULL DEFAULT '',
		  "res" text NOT NULL DEFAULT '',
		  "conditions" text NOT NULL DEFAULT '',
		  "ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_languages" ADD CONSTRAINT "%[1]d_languages_pkey" PRIMARY KEY (id);
		CREATE INDEX "%[1]d_languages_index_name" ON "%[1]d_languages" (name);
		
		DROP TABLE IF EXISTS "%[1]d_sections"; CREATE TABLE "%[1]d_sections" (
		"id" bigint  NOT NULL DEFAULT '0',
		"title" varchar(255)  NOT NULL DEFAULT '',
		"urlname" varchar(255) NOT NULL DEFAULT '',
		"page" varchar(255) NOT NULL DEFAULT '',
		"roles_access" text NOT NULL DEFAULT '',
		"delete" bigint NOT NULL DEFAULT '0',
		"ecosystem" bigint NOT NULL DEFAULT '1'
		);
	  ALTER TABLE ONLY "%[1]d_sections" ADD CONSTRAINT "%[1]d_sections_pkey" PRIMARY KEY (id);

		DROP TABLE IF EXISTS "%[1]d_menu";
		CREATE TABLE "%[1]d_menu" (
			"id" bigint  NOT NULL DEFAULT '0',
			"name" character varying(255) UNIQUE NOT NULL DEFAULT '',
			"title" character varying(255) NOT NULL DEFAULT '',
			"value" text NOT NULL DEFAULT '',
			"conditions" text NOT NULL DEFAULT '',
			"ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_menu" ADD CONSTRAINT "%[1]d_menu_pkey" PRIMARY KEY (id);
		CREATE INDEX "%[1]d_menu_index_name" ON "%[1]d_menu" (name);

		DROP TABLE IF EXISTS "%[1]d_pages"; 
		CREATE TABLE "%[1]d_pages" (
			"id" bigint  NOT NULL DEFAULT '0',
			"name" character varying(255) UNIQUE NOT NULL DEFAULT '',
			"value" text NOT NULL DEFAULT '',
			"menu" character varying(255) NOT NULL DEFAULT '',
			"validate_count" bigint NOT NULL DEFAULT '1',
			"conditions" text NOT NULL DEFAULT '',
			"app_id" bigint NOT NULL DEFAULT '1',
			"validate_mode" character(1) NOT NULL DEFAULT '0',
			"ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_pages" ADD CONSTRAINT "%[1]d_pages_pkey" PRIMARY KEY (id);
		CREATE INDEX "%[1]d_pages_index_name" ON "%[1]d_pages" (name);


		DROP TABLE IF EXISTS "%[1]d_blocks"; CREATE TABLE "%[1]d_blocks" (
			"id" bigint  NOT NULL DEFAULT '0',
			"name" character varying(255) UNIQUE NOT NULL DEFAULT '',
			"value" text NOT NULL DEFAULT '',
			"conditions" text NOT NULL DEFAULT '',
			"app_id" bigint NOT NULL DEFAULT '1',
			"ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_blocks" ADD CONSTRAINT "%[1]d_blocks_pkey" PRIMARY KEY (id);
		CREATE INDEX "%[1]d_blocks_index_name" ON "%[1]d_blocks" (name);
		
		DROP TABLE IF EXISTS "%[1]d_signatures"; CREATE TABLE "%[1]d_signatures" (
			"id" bigint  NOT NULL DEFAULT '0',
			"name" character varying(100) NOT NULL DEFAULT '',
			"value" jsonb,
			"conditions" text NOT NULL DEFAULT ''
		);
		ALTER TABLE ONLY "%[1]d_signatures" ADD CONSTRAINT "%[1]d_signatures_pkey" PRIMARY KEY (name);
		
		CREATE TABLE "%[1]d_contracts" (
		"id" bigint NOT NULL  DEFAULT '0',
		"name" text NOT NULL UNIQUE DEFAULT '',
		"value" text  NOT NULL DEFAULT '',
		"wallet_id" bigint NOT NULL DEFAULT '0',
		"token_id" bigint NOT NULL DEFAULT '1',
		"conditions" text  NOT NULL DEFAULT '',
		"app_id" bigint NOT NULL DEFAULT '1',
		"ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_contracts" ADD CONSTRAINT "%[1]d_contracts_pkey" PRIMARY KEY (id);
		
		
		DROP TABLE IF EXISTS "%[1]d_parameters";
		CREATE TABLE "%[1]d_parameters" (
		"id" bigint NOT NULL  DEFAULT '0',
		"name" varchar(255) UNIQUE NOT NULL DEFAULT '',
		"value" text NOT NULL DEFAULT '',
		"conditions" text  NOT NULL DEFAULT '',
		"ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_parameters" ADD CONSTRAINT "%[1]d_parameters_pkey" PRIMARY KEY ("id");
		CREATE INDEX "%[1]d_parameters_index_name" ON "%[1]d_parameters" (name);

		DROP TABLE IF EXISTS "%[1]d_app_params";
		CREATE TABLE "%[1]d_app_params" (
		"id" bigint NOT NULL  DEFAULT '0',
		"app_id" bigint NOT NULL  DEFAULT '0',
		"name" varchar(255) UNIQUE NOT NULL DEFAULT '',
		"value" text NOT NULL DEFAULT '',
		"conditions" text  NOT NULL DEFAULT '',
		"ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_app_params" ADD CONSTRAINT "%[1]d_app_params_pkey" PRIMARY KEY ("id");
		CREATE INDEX "%[1]d_app_params_index_name" ON "%[1]d_app_params" (name);
		CREATE INDEX "%[1]d_app_params_index_app" ON "%[1]d_app_params" (app_id);
		
		DROP TABLE IF EXISTS "%[1]d_tables";
		CREATE TABLE "%[1]d_tables" (
		"id" bigint NOT NULL  DEFAULT '0',
		"name" varchar(100) UNIQUE NOT NULL DEFAULT '',
		"permissions" jsonb,
		"columns" jsonb,
		"conditions" text  NOT NULL DEFAULT '',
		"app_id" bigint NOT NULL DEFAULT '1',
		"ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_tables" ADD CONSTRAINT "%[1]d_tables_pkey" PRIMARY KEY ("id");
		CREATE INDEX "%[1]d_tables_index_name" ON "%[1]d_tables" (name);
		
		DROP TABLE IF EXISTS "%[1]d_notifications";
		CREATE TABLE "%[1]d_notifications" (
			"id"    bigint NOT NULL DEFAULT '0',
			"recipient" jsonb,
			"sender" jsonb,
			"notification" jsonb,
			"page_params"	jsonb,
			"processing_info" jsonb,
			"page_name"	varchar(255) NOT NULL DEFAULT '',
			"date_created"	bigint NOT NULL DEFAULT '0',
			"date_start_processing" bigint NOT NULL DEFAULT '0',
			"date_closed" bigint NOT NULL DEFAULT '0',
			"closed" bigint NOT NULL DEFAULT '0',
			"ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_notifications" ADD CONSTRAINT "%[1]d_notifications_pkey" PRIMARY KEY ("id");


		DROP TABLE IF EXISTS "%[1]d_roles";
		CREATE TABLE "%[1]d_roles" (
			"id" 	bigint NOT NULL DEFAULT '0',
			"default_page"	varchar(255) NOT NULL DEFAULT '',
			"role_name"	varchar(255) NOT NULL DEFAULT '',
			"deleted"    bigint NOT NULL DEFAULT '0',
			"role_type" bigint NOT NULL DEFAULT '0',
			"creator" jsonb NOT NULL DEFAULT '{}',
			"date_created" bigint NOT NULL DEFAULT '0',
			"date_deleted" bigint NOT NULL DEFAULT '0',
			"company_id" bigint NOT NULL DEFAULT '0',
			"roles_access" jsonb, 
			"image_id" bigint NOT NULL DEFAULT '0',
			"ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_roles" ADD CONSTRAINT "%[1]d_roles_pkey" PRIMARY KEY ("id");
		CREATE INDEX "%[1]d_roles_index_deleted" ON "%[1]d_roles" (deleted);
		CREATE INDEX "%[1]d_roles_index_type" ON "%[1]d_roles" (role_type);


		DROP TABLE IF EXISTS "%[1]d_roles_participants";
		CREATE TABLE "%[1]d_roles_participants" (
			"id" bigint NOT NULL DEFAULT '0',
			"role" jsonb,
			"member" jsonb,
			"appointed" jsonb,
			"date_created" bigint NOT NULL DEFAULT '0',
			"date_deleted" bigint NOT NULL DEFAULT '0',
			"deleted" bigint NOT NULL DEFAULT '0',
			"ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_roles_participants" ADD CONSTRAINT "%[1]d_roles_participants_pkey" PRIMARY KEY ("id");


		DROP TABLE IF EXISTS "%[1]d_members";
		CREATE TABLE "%[1]d_members" (
			"id" bigint NOT NULL DEFAULT '0',
			"member_name"	varchar(255) NOT NULL DEFAULT '',
			"image_id"	bigint NOT NULL DEFAULT '0',
			"member_info"   jsonb,
			"ecosystem" bigint NOT NULL DEFAULT '1',
			"account" char(24) NOT NULL
		);
		ALTER TABLE ONLY "%[1]d_members" ADD CONSTRAINT "%[1]d_members_pkey" PRIMARY KEY ("id");
		CREATE INDEX "%[1]d_members_index_ecosystem" ON "1_sections" (ecosystem);
		CREATE UNIQUE INDEX "%[1]d_members_uindex_ecosystem_account" ON "1_members" (account, ecosystem);

		DROP TABLE IF EXISTS "%[1]d_applications";
		CREATE TABLE "%[1]d_applications" (
			"id" bigint NOT NULL DEFAULT '0',
			"name" varchar(255) NOT NULL DEFAULT '',
			"uuid" uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
			"conditions" text NOT NULL DEFAULT '',
			"deleted" bigint NOT NULL DEFAULT '0',
			"ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_applications" ADD CONSTRAINT "%[1]d_application_pkey" PRIMARY KEY ("id");

		DROP TABLE IF EXISTS "%[1]d_binaries";
		CREATE TABLE "%[1]d_binaries" (
			"id" bigint NOT NULL DEFAULT '0',
			"app_id" bigint NOT NULL DEFAULT '1',
			"name" varchar(255) NOT NULL DEFAULT '',
			"data" bytea NOT NULL DEFAULT '',
			"hash" varchar(32) NOT NULL DEFAULT '',
			"mime_type" varchar(255) NOT NULL DEFAULT '',
			"ecosystem" bigint NOT NULL DEFAULT '1',
			"account" char(24) NOT NULL
		);
		ALTER TABLE ONLY "%[1]d_binaries" ADD CONSTRAINT "%[1]d_binaries_pkey" PRIMARY KEY (id);
		CREATE UNIQUE INDEX "%[1]d_binaries_uindex" ON "%[1]d_binaries" (account, ecosystem, app_id, name);
		
		DROP TABLE IF EXISTS "%[1]d_buffer_data";
		CREATE TABLE "%[1]d_buffer_data" (
			"id" bigint NOT NULL DEFAULT '0',
			"key" varchar(255) NOT NULL DEFAULT '',
			"value" jsonb,
			"ecosystem" bigint NOT NULL DEFAULT '1',
			"account" char(24) NOT NULL
		);
		ALTER TABLE ONLY "%[1]d_buffer_data" ADD CONSTRAINT "%[1]d_buffer_data_pkey" PRIMARY KEY ("id");

		DROP TABLE IF EXISTS "%[1]d_platform_parameters";
		CREATE TABLE "%[1]d_platform_parameters" (
		"id" bigint NOT NULL DEFAULT '0',
		"name" varchar(255)  NOT NULL DEFAULT '',
		"value" text NOT NULL DEFAULT '',
		"conditions" text  NOT NULL DEFAULT '',
		"ecosystem" bigint NOT NULL DEFAULT '1'
		);
		ALTER TABLE ONLY "%[1]d_platform_parameters" ADD CONSTRAINT "%[1]d_platform_parameters_pkey" PRIMARY KEY (id);
		CREATE INDEX "%[1]d_platform_parameters_index_name" ON "%[1]d_platform_parameters" (name);

		DROP TABLE IF EXISTS "%[1]d_cron";
	  CREATE TABLE "%[1]d_cron" (
		  "id"        bigint NOT NULL DEFAULT '0',
		  "owner"	  bigint NOT NULL DEFAULT '0',
		  "cron"      varchar(255) NOT NULL DEFAULT '',
		  "contract"  varchar(255) NOT NULL DEFAULT '',
		  "counter"   bigint NOT NULL DEFAULT '0',
		  "till"      timestamp NOT NULL DEFAULT timestamp '1970-01-01 00:00:00',
		  "conditions" text  NOT NULL DEFAULT ''
	  );
	  ALTER TABLE ONLY "%[1]d_cron" ADD CONSTRAINT "%[1]d_cron_pkey" PRIMARY KEY ("id");
	
`
