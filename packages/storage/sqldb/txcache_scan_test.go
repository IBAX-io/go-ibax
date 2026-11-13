/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// The regular expressions the cache used, the references of leadingWord and writtenTable
var (
	leadingWordRE    = regexp.MustCompile(`^\w+`)
	writeStatementRE = regexp.MustCompile(`(?is)^\s*(?:insert\s+into|update|delete\s+from)\s+(?:only\s+)?(?:(?:"[^"]+"|\w+)\s*\.\s*)*("[^"]+"|\w+)`)
)

// statementParts expands the bytes of a fuzz input to parts of statements, so that the inputs are
// writes often
var statementParts = []string{"insert", "INTO", "update", "Delete", "from", "only", "ONLY", "inſert",
	"INSERT ", "UPDATE ", "DELETE FROM ", "\"", "\"1_keys\"", "\"a\"\"b\"", ".", " . ", " ", "\t", "\n",
	"\f", "\r", "\v", " ", "ſ", "K", "s", "k", "1_keys", "public", "_", "x9", "é", "\xff",
	"\xc5", "\xe2\x84", ";", "(", "$1", "SELECT ", "savepoint ", "release ", "�", "into ", "from "}

func expandStatement(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		b.WriteString(statementParts[int(s[i])%len(statementParts)])
	}
	return b.String()
}

func checkStatementScanners(t *testing.T, s string) {
	t.Helper()
	if got, want := leadingWord(s), leadingWordRE.FindString(s); got != want {
		t.Fatalf("leadingWord(%q) = %q, want %q", s, got, want)
	}
	table, ok := writtenTable(s)
	m := writeStatementRE.FindStringSubmatch(s)
	if ok != (m != nil) || ok && table != m[1] {
		t.Fatalf("writtenTable(%q) = %q, %v; the regular expression gives %q", s, table, ok, m)
	}
}

func TestStatementScanners(t *testing.T) {
	for _, s := range []string{
		"", " ", "insert", "update", "update x", "UPDATE \"1_keys\" SET x = 1", "update\t\n\f\rx",
		"update\vx", "update x", " \t update x", "updatex", "update  only x", "update only",
		"update only ", "update only x", "update only.x", "update only . x", "update only \"\"",
		"update only \"", "update ONLY \"x\".\"y\"", "update a.b.c", "update a . b . c", "update a.",
		"update a. ", "update a.b.", "update a.\"\"", "update a.\"", "update \"a.b\".c", "update \"\"",
		"update \"\".x", "update \"x", "update \"x\"y", "update \"x\"\"y\"", "update \"x\"..y",
		"update a..b", "update .a", "update a;b", "insert into x", "insert  into x", "insertinto x",
		"insert into", "insert into ", "insert into only x", "INSERT INTO public.\"1_keys\" (id)",
		"inſert into x", "INſERT INTO x", "insert into ſ", "insert into K.x",
		"insert into xK", "insert into xKy", "update ſſ", "delete from x",
		"delete  from  x", "deletefrom x", "delete x", "DELETE FROM ONLY \"x\"", "delete from \xff",
		"delete from x\xff", "delete from \"\xff\"", "delete from \"\xff", "update xé",
		"update é", "update �", "update x\x00", "select * from x", "with x as (update y)",
		"updateſ x", "uſdate x", "update x only", "update only only", "update only only x",
		"update only.only", "update only .", "update \"only\" x", "1update x", "_update x",
	} {
		checkStatementScanners(t, s)
	}
}

// TestFoldedWordLen checks every character against \w under (?i)
func TestFoldedWordLen(t *testing.T) {
	re := regexp.MustCompile(`(?i)^\w$`)
	for r := rune(0); r <= utf8.MaxRune; r++ {
		if !utf8.ValidRune(r) {
			continue
		}
		s := string(r)
		want := 0
		if re.MatchString(s) {
			want = len(s)
		}
		if got := foldedWordLen(s, 0); got != want {
			t.Fatalf("foldedWordLen(%q) = %d, want %d", s, got, want)
		}
	}
}

func FuzzStatementScanners(f *testing.F) {
	for _, s := range []string{
		`INSERT INTO "1_keys" (id) VALUES ($1)`, `update only public."1_app_params" set value = $1`,
		`UPDATE public . "1_tables" SET columns = $1`, `delete from public.info_block`,
		"inſert into K", `update a.`, `update only . x`, `SAVEPOINT "tx-1"`, "", "\xff",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		checkStatementScanners(t, s)
		checkStatementScanners(t, expandStatement(s))
	})
}

var benchStatements = []string{
	`SELECT "id", "value" FROM "1_parameters" WHERE "name" = $1`,
	`INSERT INTO "1_keys" ("id","amount") VALUES ($1,$2)`,
	`UPDATE "1_keys" SET "amount" = $1 WHERE "id" = $2`,
	`update only public."1_app_params" set value = $1`,
	`DELETE FROM "rollback_tx" WHERE "block_id" = $1`,
	`SAVEPOINT "tx-1"`,
}

func BenchmarkWrittenScanner(b *testing.B) {
	for b.Loop() {
		for _, s := range benchStatements {
			if w := leadingWord(s); w != "SELECT" && w != "SAVEPOINT" {
				writtenTable(s)
			}
		}
	}
}

func BenchmarkWrittenRegexp(b *testing.B) {
	for b.Loop() {
		for _, s := range benchStatements {
			if w := leadingWordRE.FindString(s); w != "SELECT" && w != "SAVEPOINT" {
				writeStatementRE.FindStringSubmatch(s)
			}
		}
	}
}
