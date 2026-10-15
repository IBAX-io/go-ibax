/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package queryBuilder

import (
	"strings"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
)

type fixedID int64

func (id fixedID) GetNextID(string) (int64, error) { return int64(id), nil }

func jsonBuilder(carried func(string) bool) *SQLQueryBuilder {
	return &SQLQueryBuilder{
		Entry:        log.WithFields(log.Fields{"mod": "test"}),
		Table:        "1_roles_participants",
		Fields:       []string{"ecosystem", "member->name", "role->id"},
		FieldValues:  []any{"16", "founder", "31"},
		Where:        types.LoadMap(map[string]any{`id`: `110`}),
		KeyTableChkr: TestKeyTableChecker{false},
		Carried:      carried,
	}
}

func TestJSONColumnsSorted(t *testing.T) {
	q, err := jsonBuilder(nil).GetSQLInsertQuery(fixedID(110))
	if err != nil {
		t.Fatal(err)
	}
	want := `INSERT INTO "1_roles_participants" ("ecosystem",member,role,id) VALUES ('16','{"name":"founder"}'::jsonb,'{"id":"31"}'::jsonb,'110')`
	if q != want {
		t.Fatalf("got %s\nwant %s", q, want)
	}
}

func TestJSONColumnsCarried(t *testing.T) {
	carried := func(s string) bool {
		return strings.Contains(s, "role,member") || strings.Contains(s, `role=`+`'{}'::jsonb || '{"id":"31"}'::jsonb,member=`)
	}
	q, err := jsonBuilder(carried).GetSQLInsertQuery(fixedID(110))
	if err != nil {
		t.Fatal(err)
	}
	want := `INSERT INTO "1_roles_participants" ("ecosystem",role,member,id) VALUES ('16','{"id":"31"}'::jsonb,'{"name":"founder"}'::jsonb,'110')`
	if q != want {
		t.Fatalf("got %s\nwant %s", q, want)
	}
	expr, err := jsonBuilder(carried).GetSQLUpdateExpr(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	wantExpr := `"ecosystem"='16',role='{}'::jsonb || '{"id":"31"}'::jsonb,member='{}'::jsonb || '{"name":"founder"}'::jsonb`
	if expr != wantExpr {
		t.Fatalf("got %s\nwant %s", expr, wantExpr)
	}
}
