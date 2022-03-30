/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package queryBuilder

import (
	"fmt"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/types"

	log "github.com/sirupsen/logrus"
)

// query="SELECT ,,,id,amount,\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\"ecosystem\"
// FROM \"1_keys\" \nWHERE  AND id = -6752330173818123413 AND ecosystem = '1'\n"

// fields="[+amount]"
// values="[2912910000000]"

// whereF="[id]"
// whereV="[-6752330173818123413]"

type TestKeyTableChecker struct {
	Val bool
}

func (tc TestKeyTableChecker) IsKeyTable(tableName string) bool {
	return tc.Val
}
func TestSqlFields(t *testing.T) {
	qb := SQLQueryBuilder{
		Entry:        log.WithFields(log.Fields{"mod": "test"}),
		Table:        "1_keys",
		Fields:       []string{"+amount"},
		FieldValues:  []any{2912910000000},
		Where:        types.LoadMap(map[string]any{`id`: `-6752330173818123413`}),
		KeyTableChkr: TestKeyTableChecker{true},
	}

	fields, err := qb.GetSQLSelectFieldsExpr()
	if err != nil {
		t.Error(err)
		return
	}

	fmt.Println(fields)
}
