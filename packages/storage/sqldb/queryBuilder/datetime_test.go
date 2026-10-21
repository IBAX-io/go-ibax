/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package queryBuilder

import (
	"errors"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/types"

	log "github.com/sirupsen/logrus"
)

// withDateTimeColumns makes the columns of the table 1_events date and time columns
func withDateTimeColumns(t *testing.T) {
	t.Helper()
	saved := syspar.GetTableColType()
	syspar.SetTableColType(map[string]map[string]string{`1_events`: {
		`at`:      `timestamp without time zone`,
		`at_tz`:   `timestamp with time zone`,
		`day`:     `date`,
		`opens`:   `time without time zone`,
		`payload`: `bytea`,
	}})
	t.Cleanup(func() { syspar.SetTableColType(saved) })
}

// nondeterministic are the values the database reads as the time of the node executing the statement
var nondeterministic = []string{`now`, `today`, `tomorrow`, `yesterday`, ` NOW `, `epoch`, `infinity`,
	`2024-01-01 now`, `allballs`, `2024-01-01 12:00:00+08`}

func TestCheckDateTimeValue(t *testing.T) {
	for _, c := range []struct {
		dataType string
		value    string
		ok       bool
	}{
		{`timestamp without time zone`, `2024-02-29`, true},
		{`timestamp without time zone`, `2024-02-29 13:05`, true},
		{`timestamp without time zone`, `2024-02-29T13:05:07`, true},
		{`timestamp with time zone`, `2024-02-29 13:05:07.123456`, true},
		{`date`, `2024-02-29`, true},
		{`time without time zone`, `13:05:07`, true},
		{`time with time zone`, `13:05`, true},
		{`time without time zone`, `2024-02-29`, false},
		{`timestamp without time zone`, `2024-02-29 13:05:07.1234567`, false},
		{`text`, `now`, true},
		{``, `today`, true},
		{`bytea`, `now`, true},
	} {
		if err := checkDateTimeValue(c.dataType, c.value); (err == nil) != c.ok {
			t.Errorf("%s %q: %v", c.dataType, c.value, err)
		}
	}
	for _, dataType := range []string{`timestamp without time zone`, `date`, `time without time zone`} {
		for _, v := range nondeterministic {
			if err := checkDateTimeValue(dataType, v); !errors.Is(err, ErrDateTimeValue) {
				t.Errorf("%s %q: %v", dataType, v, err)
			}
		}
	}
	for v, ok := range map[string]bool{`1700000000`: true, `-1.5`: true, `now`: false, `1e9`: false, ``: false} {
		if err := checkUnixTime(v); (err == nil) != ok {
			t.Errorf("unix %q: %v", v, err)
		}
	}
}

// A value compared with a date or time column must be a literal, wherever it is in the condition
func TestGetWhereDateTime(t *testing.T) {
	withDateTimeColumns(t)
	for _, c := range []struct {
		name  string
		where *types.Map
	}{
		{`eq`, where(`at`, `now`)},
		{`$gt`, where(`at`, where(`$gt`, `today`))},
		{`$lte date`, where(`day`, where(`$lte`, `tomorrow`))},
		{`$in`, where(`at_tz`, where(`$in`, []any{`2024-01-01`, `yesterday`}))},
		{`$nin string`, where(`opens`, where(`$nin`, `now`))},
		{`list`, where(`at`, []any{`2024-01-01`, `now`})},
		{`list map`, where(`at`, []any{where(`$neq`, `now`)})},
		{`$or`, where(`$or`, []any{where(`name`, `x`), where(`at`, where(`$lt`, `now`))})},
		{`column $or`, where(`at`, where(`$or`, []any{where(`$gt`, `2024-01-01`), where(`$lt`, `now`)}))},
	} {
		if _, _, err := GetWhere(`1_events`, c.where); !errors.Is(err, ErrDateTimeValue) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	for _, c := range []struct {
		name  string
		where *types.Map
	}{
		{`literal`, where(`at`, `2024-01-01 10:00:00`)},
		{`$gte`, where(`day`, where(`$gte`, `2024-01-01`))},
		{`isnull`, where(`at`, `$isnull`)},
		{`other column`, where(`name`, `now`)},
		{`other table`, where(`at`, `now`)},
		{`json`, where(`at->x`, `now`)},
		{`like`, where(`name`, where(`$like`, `now`))},
	} {
		table := `1_events`
		if c.name == `other table` {
			table = `1_other`
		}
		if _, _, err := GetWhere(table, c.where); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func dateTimeBuilder(fields []string, values []any) *SQLQueryBuilder {
	return &SQLQueryBuilder{
		Entry:        log.WithFields(log.Fields{"mod": "test"}),
		Table:        `1_events`,
		Fields:       fields,
		FieldValues:  values,
		Where:        types.LoadMap(map[string]any{`id`: `1`}),
		KeyTableChkr: TestKeyTableChecker{false},
	}
}

type testIDGetter struct{}

func (testIDGetter) GetNextID(string) (int64, error) { return 1, nil }

// An updated or inserted value of a date or time column must be a literal
func TestSetDateTime(t *testing.T) {
	withDateTimeColumns(t)
	for _, c := range []struct {
		field string
		value any
		ok    bool
	}{
		{`at`, `2024-01-01 10:00:00`, true},
		{`at`, `now`, false},
		{`day`, `today`, false},
		{`opens`, `allballs`, false},
		{`at`, `NULL`, true},
		{`name`, `now`, true},
		{`timestamp at`, `1700000000`, true},
		{`timestamp at`, `now`, false},
		{`name`, `timestamp 2024-01-01`, true},
		{`name`, `timestamp now`, false},
		{`name`, `timestamp`, true},
	} {
		_, _, err := dateTimeBuilder([]string{c.field}, []any{c.value}).GetSQLUpdateExpr(nil)
		if (err == nil) != c.ok {
			t.Errorf("update %s=%v: %v", c.field, c.value, err)
		}
		_, _, err = dateTimeBuilder([]string{c.field}, []any{c.value}).GetSQLInsertQuery(testIDGetter{})
		if (err == nil) != c.ok {
			t.Errorf("insert %s=%v: %v", c.field, c.value, err)
		}
	}
}

// The statements of the date and time values keep their text
func TestSetDateTimeStatements(t *testing.T) {
	withDateTimeColumns(t)
	b := dateTimeBuilder([]string{`timestamp at`, `day`, `name`}, []any{`1700000000`, `2024-01-01`, `timestamp 2024-01-01 10:00`})
	set, args, err := b.GetSQLUpdateExpr(nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := `at= to_timestamp(?),"day"=?,name= ?::timestamp`; set != want {
		t.Errorf("update %s, want %s", set, want)
	}
	if len(args) != 3 || args[2] != `2024-01-01 10:00` {
		t.Errorf("update args %q", args)
	}
	b = dateTimeBuilder([]string{`timestamp at`, `day`, `name`}, []any{`1700000000`, `2024-01-01`, `timestamp 2024-01-01 10:00`})
	query, _, err := b.GetSQLInsertQuery(testIDGetter{})
	if err != nil {
		t.Fatal(err)
	}
	if want := `INSERT INTO "1_events" (at,"day","name",id) VALUES (to_timestamp(?),?,?::timestamp,?)`; query != want {
		t.Errorf("insert %s, want %s", query, want)
	}
}
