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
		
		
		CREATE TABLE "system_tables" (
		"name" varchar(100)  NOT NULL DEFAULT '',
		"permissions" jsonb,
		"columns" jsonb,
		"conditions" text  NOT NULL DEFAULT ''
		);
		ALTER TABLE ONLY "system_tables" ADD CONSTRAINT system_tables_pkey PRIMARY KEY (name);
		
		DROP TABLE IF EXISTS "install"; CREATE TABLE "install" (
		"progress" varchar(10) NOT NULL DEFAULT ''
		);
		
		DROP TABLE IF EXISTS "stop_daemons"; CREATE TABLE "stop_daemons" (
		"stop_time" int NOT NULL DEFAULT '0'
		);`
)
