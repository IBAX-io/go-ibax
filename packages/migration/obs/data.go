/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package obs

var (
	migrationInitial = `
		DROP SEQUENCE IF EXISTS migration_history_id_seq CASCADE;
		CREATE SEQUENCE migration_history_id_seq START WITH 1;
		DROP TABLE IF EXISTS "migration_history";
		CREATE TABLE "migration_history" (
			"id" int NOT NULL default nextval('migration_history_id_seq'),
			"version" varchar(255) NOT NULL,
			"date_applied" int NOT NULL
		);
		ALTER SEQUENCE migration_history_id_seq owned by migration_history.id;
		ALTER TABLE ONLY "migration_history" ADD CONSTRAINT migration_history_pkey PRIMARY KEY (id);`

	migrationInitialSchema = `
		CREATE TABLE "system_contracts" (
		"id" bigint NOT NULL  DEFAULT '0',
		"value" text  NOT NULL DEFAULT '',
		"wallet_id" bigint NOT NULL DEFAULT '0',
		"token_id" bigint NOT NULL DEFAULT '0',
		"active" character(1) NOT NULL DEFAULT '0',
		"progress" varchar(10) NOT NULL DEFAULT ''
		);
		
		DROP TABLE IF EXISTS "stop_daemons"; CREATE TABLE "stop_daemons" (
		"stop_time" int NOT NULL DEFAULT '0'
		);`
)
