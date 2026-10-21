/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package rollback

import (
	"reflect"
	"strings"
	"testing"
)

func TestRollbackAssignments(t *testing.T) {
	var asked []string
	isByte := func(column string) bool {
		asked = append(asked, column)
		return column == "pub"
	}
	set, args, err := rollbackAssignments(map[string]string{`"pub"`: "0a0b", "name": "it's", "amount": "NULL"}, isByte)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"pub"=decode(?,'hex'),"amount"=NULL,"name"=?`; set != want {
		t.Errorf("set %s, want %s", set, want)
	}
	if want := []any{"0a0b", "it's"}; !reflect.DeepEqual(args, want) {
		t.Errorf("args %q, want %q", args, want)
	}
	for _, column := range asked {
		if strings.Contains(column, `"`) {
			t.Errorf("isByte was asked for the quoted column %s", column)
		}
	}
	if _, _, err := rollbackAssignments(map[string]string{"pub": "0a0b','x"}, isByte); err == nil {
		t.Error("rollback data of a bytea column that is not hex was taken")
	}
}
