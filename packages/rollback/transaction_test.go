/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package rollback

import (
	"strings"
	"testing"
)

func TestRollbackAssignments(t *testing.T) {
	var asked []string
	isByte := func(column string) bool {
		asked = append(asked, column)
		return column == "pub"
	}
	set, err := rollbackAssignments(map[string]string{`"pub"`: "0a0b", "name": "it's", "amount": "NULL"}, isByte)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"pub"=decode('0a0b','hex')`, `"name"='it''s'`, `"amount"=NULL`} {
		if !strings.Contains(set, want) {
			t.Errorf("%s is not in %s", want, set)
		}
	}
	for _, column := range asked {
		if strings.Contains(column, `"`) {
			t.Errorf("isByte was asked for the quoted column %s", column)
		}
	}
	if _, err := rollbackAssignments(map[string]string{"pub": "0a0b','x"}, isByte); err == nil {
		t.Error("rollback data of a bytea column that is not hex was taken")
	}
}
