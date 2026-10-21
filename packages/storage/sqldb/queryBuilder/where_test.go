/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package queryBuilder

import (
	"reflect"
	"strings"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/types"

	log "github.com/sirupsen/logrus"
)

// value is a value that would end the string literal it was put in
const value = `a'b"?@--`

func where(key string, v any) *types.Map {
	m := types.NewMap()
	m.Set(key, v)
	return m
}

func noValue(t *testing.T, name, sql string) {
	t.Helper()
	if strings.Contains(sql, `a'b`) || strings.Contains(sql, `@`) {
		t.Errorf("%s: a value is in the statement %s", name, sql)
	}
}

// Every value of a condition is bound to a placeholder and never put in the condition
func TestGetWhereBindsValues(t *testing.T) {
	for _, c := range []struct {
		name  string
		where *types.Map
		sql   string
		args  []any
	}{
		{`eq`, where(`name`, value), `"name" = ?`, []any{value}},
		{`$eq`, where(`name`, where(`$eq`, value)), `("name" = ?)`, []any{value}},
		{`$neq`, where(`name`, where(`$neq`, value)), `("name" != ?)`, []any{value}},
		{`$gt`, where(`amount`, where(`$gt`, 5)), `("amount" > ?)`, []any{`5`}},
		{`$lte`, where(`amount`, where(`$lte`, value)), `("amount" <= ?)`, []any{value}},
		{`$like`, where(`name`, where(`$like`, value)), `("name" like ?)`, []any{`%` + value + `%`}},
		{`$ibegin`, where(`name`, where(`$ibegin`, value)), `("name" ilike ?)`, []any{value + `%`}},
		{`$end`, where(`name`, where(`$end`, value)), `("name" like ?)`, []any{`%` + value}},
		{`$in string`, where(`name`, where(`$in`, value)), `("name" in (?))`, []any{value}},
		{`$nin list`, where(`name`, where(`$nin`, []any{value, 7})), `("name" not in (?,?))`, []any{value, `7`}},
		{`$in empty`, where(`name`, where(`$in`, []any{})), `false`, nil},
		{`list`, where(`name`, []any{value, where(`$neq`, `x`)}), `("name" = ? and ("name" != ?))`, []any{value, `x`}},
		{`$or`, where(`$or`, []any{where(`a`, value), where(`b`, where(`$lt`, 1))}),
			`("a" = ? or ("b" < ?))`, []any{value, `1`}},
		{`bytes`, where(`pub`, []byte{0x0a, 0x27}), `"pub" = decode(?,'HEX')`, []any{`0a27`}},
		{`isnull`, where(`name`, `$isnull`), `"name" is null`, nil},
		{`json`, where(`doc->title`, value), `doc->>'title' = ?`, []any{value}},
	} {
		sql, args, err := GetWhere(``, c.where)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		noValue(t, c.name, sql)
		if sql != c.sql {
			t.Errorf("%s: %s, want %s", c.name, sql, c.sql)
		}
		if strings.Count(sql, `?`) != len(args) || !reflect.DeepEqual(args, c.args) {
			t.Errorf("%s: args %q, want %q", c.name, args, c.args)
		}
	}
}

// The values of an update and of an insert are bound to placeholders, in the order of them
func TestBuilderBindsValues(t *testing.T) {
	newBuilder := func() *SQLQueryBuilder {
		return &SQLQueryBuilder{
			Entry:        log.WithFields(log.Fields{"mod": "test"}),
			Table:        "1_items",
			Fields:       []string{"name", "+amount", "doc->title", "timestamp created", "note"},
			FieldValues:  []any{value, "5", value, "1700000000", "NULL"},
			Where:        where(`name`, value),
			KeyTableChkr: TestKeyTableChecker{false},
		}
	}

	b := newBuilder()
	set, setArgs, err := b.GetSQLUpdateExpr(nil)
	if err != nil {
		t.Fatal(err)
	}
	whereExpr, whereArgs, err := b.GetSQLWhereExpr()
	if err != nil {
		t.Fatal(err)
	}
	update := `UPDATE "` + b.Table + `" SET ` + set + whereExpr
	noValue(t, "update", update)
	want := `"name"=?,amount=amount+?::numeric,created= to_timestamp(?),note= NULL,doc='{}'::jsonb || ?::jsonb`
	if set != want {
		t.Errorf("set %s, want %s", set, want)
	}
	wantArgs := []any{value, "5", "1700000000", `{"title":"a'b\"?@--"}`}
	if !reflect.DeepEqual(setArgs, wantArgs) {
		t.Errorf("set args %q, want %q", setArgs, wantArgs)
	}
	if whereExpr != ` WHERE "name" = ?` || !reflect.DeepEqual(whereArgs, []any{value}) {
		t.Errorf("where %s %q", whereExpr, whereArgs)
	}

	b = newBuilder()
	insert, insertArgs, err := b.GetSQLInsertQuery(nextID(9))
	if err != nil {
		t.Fatal(err)
	}
	noValue(t, "insert", insert)
	want = `INSERT INTO "1_items" ("name",amount,created,"note",doc,id) VALUES (?,?,to_timestamp(?),NULL,?::jsonb,?)`
	if insert != want {
		t.Errorf("insert %s, want %s", insert, want)
	}
	wantArgs = []any{value, "5", "1700000000", `{"title":"a'b\"?@--"}`, "9"}
	if !reflect.DeepEqual(insertArgs, wantArgs) {
		t.Errorf("insert args %q, want %q", insertArgs, wantArgs)
	}
	if b.TableID() != "9" {
		t.Errorf("table id %s", b.TableID())
	}
}

type nextID int64

func (n nextID) GetNextID(string) (int64, error) {
	return int64(n), nil
}

// A statement is hashed together with its values, so two statements that differ in a value only
// are not hashed alike
func TestBoundStatement(t *testing.T) {
	a := BoundStatement(`UPDATE "t" SET "v"=?`, []any{`1`})
	b := BoundStatement(`UPDATE "t" SET "v"=?`, []any{`2`})
	if a == b {
		t.Errorf("%s is %s", a, b)
	}
	if got := BoundStatement(`DELETE FROM "t"`, nil); got != `DELETE FROM "t"` {
		t.Errorf("statement without values: %s", got)
	}
}
