/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"testing"

	"database/sql"

	"time"

	"context"

	"github.com/IBAX-io/go-ibax/packages/model"
)

func createTables(t *testing.T, db *sql.DB) {
	sql := `
	CREATE TABLE "main_lock" (
		"lock_time" integer NOT NULL DEFAULT '0',
		"script_name" string NOT NULL DEFAULT '',
		"info" text NOT NULL DEFAULT '',
		"uniq" integer NOT NULL DEFAULT '0'
	);
	CREATE TABLE "install" (
		"progress" text NOT NULL DEFAULT ''
	);
	`
	var err error
	_, err = db.Exec(sql)
	if err != nil {
		t.Fatalf("%s", err)
	}
}

func TestWait(t *testing.T) {
	db := initGorm(t)
	createTables(t, db.DB())

	ctx, cf := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer func() {
		ctx.Done()
		cf()
	}()

	err := WaitDB(ctx)
	if err == nil {
		t.Errorf("should be error")
	}

	install := &model.Install{}
	install.Progress = "complete"
	err = install.Create()
	if err != nil {
		t.Fatalf("save failed: %s", err)
	}

	ctx, scf := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer func() {
		ctx.Done()
		scf()
	}()

	err = WaitDB(ctx)
	if err != nil {
		t.Errorf("wait failed: %s", err)
	}
}
