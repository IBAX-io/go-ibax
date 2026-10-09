/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

import (
	"regexp"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

var tableRow = regexp.MustCompile(`next_id\('1_tables'\),\s*'([a-z_0-9]+)',\s*'(\{[^']*\})'`)

// Every table of a new chain declares who reads its rows (docs/api/data-query.md)
func TestEveryTableDeclaresItsReaders(t *testing.T) {
	for name, data := range map[string]string{"tables": tablesDataSQL, "first tables": firstTablesDataSQL} {
		rows := tableRow.FindAllStringSubmatch(data, -1)
		if len(rows) == 0 {
			t.Fatalf("%s: no tables", name)
		}
		for _, row := range rows {
			if !regexp.MustCompile(`"read":\s*"`).MatchString(row[2]) {
				t.Errorf("%s: table %s has no read permission", name, row[1])
			}
		}
	}
}

// Every ecosystem declares, under its own id, each shared table it keeps rows in
func TestEveryEcosystemDeclaresItsTables(t *testing.T) {
	script, err := GetTableScript(SqlData{Ecosystem: 2})
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, row := range regexp.MustCompile(`next_id\('1_tables'\),\s*'([a-z_0-9]+)',[^;]*?'2'\s*\)`).FindAllStringSubmatch(script, -1) {
		declared[row[1]] = true
	}
	for name := range converter.FirstEcosystemTables {
		if name != "tables" && !declared[name] {
			t.Errorf("table %s is not declared for ecosystem 2", name)
		}
	}
}
