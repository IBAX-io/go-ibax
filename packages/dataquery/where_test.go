/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package dataquery

import (
	"encoding/json"
	"errors"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSchema map[string]Type

func (f fakeSchema) column(name string) (Type, bool, bool) {
	t, found := f[name]
	return t, false, found
}

func (f fakeSchema) readable(name string) error {
	if name == "secret" {
		return ErrAccessDenied("t." + name)
	}
	return nil
}

var testSchema = fakeSchema{
	"id": TypeNumber, "name": TypeText, "amount": TypeMoney, "rate": TypeDouble, "data": TypeJSON,
	"created": TypeTimestamp, "pub": TypeBytes, "secret": TypeText,
}

func filter(t *testing.T, text string) Where {
	t.Helper()
	var w Where
	require.NoError(t, DecodeJSON([]byte(text), &w))
	return w
}

func build(t *testing.T, text string) (string, []any, error) {
	t.Helper()
	s := &statement{schema: testSchema}
	sql, err := s.where(filter(t, text), 0)
	return sql, s.args, err
}

func code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestWhereBindsEveryValue(t *testing.T) {
	sql, args, err := build(t, `{"name": "Ann", "amount": {"$gte": "10", "$lt": 20},
		"$or": [{"id": {"$in": [1, "2"]}}, {"data->kind->x": {"$begin": "a%_"}}], "created": null}`)
	require.NoError(t, err)
	assert.Equal(t, `(((("id" IN (CAST(CAST($1 AS text) AS bigint), CAST(CAST($2 AS text) AS bigint)))) OR `+
		`((CAST(jsonb_extract_path_text("data", 'kind', 'x') AS text) LIKE CAST($3 AS text) ESCAPE '\'))) AND `+
		`("amount" >= CAST(CAST($4 AS text) AS numeric) AND "amount" < CAST(CAST($5 AS text) AS numeric)) AND `+
		`"created" IS NULL AND "name" = CAST($6 AS text))`, sql)
	assert.Equal(t, []any{"1", "2", `a\%\_%`, "10", "20", "Ann"}, args)
}

func TestWhereTypesValues(t *testing.T) {
	for _, ok := range []string{
		`{"id": 9007199254740993}`, `{"amount": "1.25"}`, `{"rate": 1.5}`, `{"created": "2026-10-09T08:00:00Z"}`,
		`{"created": "2026-10-09"}`, `{"pub": "00ff"}`, `{"data": {"$eq": {"a": 1}}}`, `{"name": 5}`,
		`{"id": {"$nin": []}}`, `{"$and": []}`,
	} {
		_, _, err := build(t, ok)
		assert.NoError(t, err, ok)
	}
	for text, want := range map[string]string{
		`{"id": "1.5"}`:              "E_VALUE",
		`{"id": true}`:               "E_VALUE",
		`{"amount": "1e5"}`:          "E_VALUE",
		`{"rate": "x"}`:              "E_VALUE",
		`{"created": "yesterday"}`:   "E_VALUE",
		`{"pub": "0F"}`:              "E_VALUE",
		`{"name": {"$gt": null}}`:    "E_VALUE",
		`{"id": {"$in": 1}}`:         "E_VALUE",
		`{"id": {"$in": [null]}}`:    "E_VALUE",
		`{"name": {"$like": 1}}`:     "E_VALUE",
		`{"data": {"$gt": 1}}`:       "E_WHERE",
		`{"name": {"$regex": "a"}}`:  "E_WHERE",
		`{"name": {}}`:               "E_WHERE",
		`{"$or": {"name": "a"}}`:     "E_WHERE",
		`{"$or": ["a"]}`:             "E_WHERE",
		`{"none": 1}`:                "E_COLUMN",
		`{"Name": "a"}`:              "E_COLUMN",
		`{"name->x": "a"}`:           "E_COLUMN",
		`{"data->X": "a"}`:           "E_COLUMN",
		`{"secret": "a"}`:            "E_ACCESS_DENIED",
		`{"$or": [{"secret": "a"}]}`: "E_ACCESS_DENIED",
	} {
		_, _, err := build(t, text)
		assert.Equal(t, want, code(err), text)
	}
}

func TestWhereLimitsItsSize(t *testing.T) {
	deep := strings.Repeat(`{"$and": [`, maxDepth+2) + `{"id": 1}` + strings.Repeat(`]}`, maxDepth+2)
	_, _, err := build(t, deep)
	assert.Equal(t, "E_WHERE", code(err))
	items := strings.Repeat(`1,`, maxConditions) + `1`
	_, _, err = build(t, `{"id": {"$in": [`+items+`]}}`)
	assert.Equal(t, "E_WHERE", code(err))
}

func TestOrderNamesReadableColumnsAndEndsWithID(t *testing.T) {
	s := &statement{schema: testSchema}
	sql, err := s.order(Order{{"amount": "desc"}, {"data->k": "ASC"}})
	require.NoError(t, err)
	assert.Equal(t, `"amount" DESC, jsonb_extract_path_text("data", 'k') ASC, "id" ASC`, sql)
	sql, err = s.order(Order{{"id": "desc"}})
	require.NoError(t, err)
	assert.Equal(t, `"id" DESC`, sql)
	for order, want := range map[string]string{
		`[{"amount": "up"}]`:            "E_WHERE",
		`[{"a": "asc", "b": "asc"}]`:    "E_WHERE",
		`[{"name; drop": "asc"}]`:       "E_COLUMN",
		`[{"secret": "asc"}]`:           "E_ACCESS_DENIED",
		`[{"id\" desc, \"x": "asc"}]`:   "E_COLUMN",
		`[{"data->a'b": "asc"}]`:        "E_COLUMN",
		`[{"(select 1)": "asc"}]`:       "E_COLUMN",
		`[{"name::text": "asc"}]`:       "E_COLUMN",
		`[{"name--": "asc"}]`:           "E_COLUMN",
		`[{"nаme": "asc"}]`:             "E_COLUMN",
		`[{"data->k->": "asc"}]`:        "E_COLUMN",
		`[{"data->k->l->m->n": "asc"}]`: "",
	} {
		var o Order
		require.NoError(t, DecodeJSON([]byte(order), &o))
		_, err := (&statement{schema: testSchema}).order(o)
		assert.Equal(t, want, code(err), order)
	}
}

func TestSelectListNamesColumnsAsNoColumnIsNamed(t *testing.T) {
	refs := []ref{{column: "id", t: TypeNumber}, {column: "data", path: []string{"k"}, t: TypeJSON}}
	assert.Equal(t,
		`CAST("id" AS text) AS "#0", CAST(jsonb_extract_path("data", 'k') AS text) AS "#1"`, selectList(refs))
	for _, name := range []string{"#0", "#1"} {
		assert.False(t, identifier.MatchString(name), name)
	}
}

// Injection payloads end up as bound values, or are refused as names: never in the SQL text
var payloads = []string{
	`'`, `''`, `' OR '1'='1`, `'; DROP TABLE "1_keys"; --`, `" OR "1"="1`, `\'`, `$1`, `$$`, `$tag$x$tag$`,
	`) OR (1=1`, `1); SELECT pg_sleep(10); --`, `/* comment */`, `-- comment`, `name::text`, `1::bigint`,
	`(SELECT secret FROM "1_keys")`, `name' || (SELECT 1) || '`, `ｓｅｌｅｃｔ`, `ʼ OR 1=1`, `％`, `%' ESCAPE '`,
	"\x00", `\x27`, `E'\\''`, `1 UNION SELECT * FROM pg_user`,
}

func TestPayloadsNeverReachTheSQLText(t *testing.T) {
	for _, given := range payloads {
		// Marked, so that a payload as short as a quote is told apart from the SQL around it
		payload := "zz" + given
		quoted, _ := json.Marshal(payload)
		for _, text := range []string{
			`{"name": ` + string(quoted) + `}`,
			`{"name": {"$like": ` + string(quoted) + `}}`,
			`{"name": {"$in": [` + string(quoted) + `]}}`,
			`{"data->k": {"$neq": ` + string(quoted) + `}}`,
		} {
			sql, args, err := build(t, text)
			require.NoError(t, err, text)
			assert.NotContains(t, sql, payload, text)
			assert.NotEmpty(t, args)
		}
		for _, text := range []string{
			`{` + string(quoted) + `: 1}`,
			`{"data->` + strings.Trim(string(quoted), `"`) + `": 1}`,
		} {
			_, _, err := build(t, text)
			if err == nil {
				t.Fatalf("%s: a payload as a name was taken", text)
			}
		}
	}
}

// Random filters never panic, and whatever SQL they give holds no string the client wrote outside
// its bound values
func TestRandomFiltersBindEveryString(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	alphabet := []rune(`abcdefghijklmnopqrstuvwxyz_0123456789'"$-;:()/*%\ ` + " ｓ")
	word := func() string {
		runes := make([]rune, 1+random.Intn(12))
		for i := range runes {
			runes[i] = alphabet[random.Intn(len(alphabet))]
		}
		return "q" + string(runes)
	}
	columns := []string{"id", "name", "amount", "rate", "data", "data->k", "created", "pub", "none", "secret"}
	operators := []string{"$eq", "$neq", "$gt", "$gte", "$lt", "$lte", "$in", "$nin", "$like", "$begin", "$end",
		"$ilike", "$ibegin", "$iend", "$x"}
	var value func(depth int) any
	value = func(depth int) any {
		switch random.Intn(6) {
		case 0:
			return word()
		case 1:
			return json.Number("12")
		case 2:
			return nil
		case 3:
			return []any{word(), json.Number("1")}
		case 4:
			if depth < 3 {
				return map[string]any{operators[random.Intn(len(operators))]: value(depth + 1)}
			}
		}
		return json.Number("1.5")
	}
	var where func(depth int) Where
	where = func(depth int) Where {
		w := Where{}
		for range random.Intn(4) {
			switch random.Intn(5) {
			case 0:
				if depth < 4 {
					w["$or"] = []any{map[string]any(where(depth + 1)), map[string]any(where(depth + 1))}
					continue
				}
			case 1:
				w[word()] = value(0)
				continue
			}
			w[columns[random.Intn(len(columns))]] = value(0)
		}
		return w
	}
	runs := 100_000
	if testing.Short() {
		runs = 10_000
	}
	built := 0
	for range runs {
		w := where(0)
		s := &statement{schema: testSchema}
		sql, err := s.where(w, 0)
		if err != nil {
			continue
		}
		built++
		for _, arg := range s.args {
			if text := arg.(string); strings.HasPrefix(text, "q") && strings.Contains(sql, text) {
				t.Fatalf("%q is in %s", text, sql)
			}
		}
	}
	assert.Greater(t, built, runs/10)
}
