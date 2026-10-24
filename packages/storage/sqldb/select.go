/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/metrics"

	"gorm.io/gorm/utils"
)

var rowStatements = metrics.DBStatements.WithLabelValues("row")

// Select is a select of the rows of a table the contracts make: the text gorm would make of it,
// without the clauses, the clones of its statement and the callbacks gorm allocates for each
// statement
type Select struct {
	Table   string
	Columns string
	// Where is a condition with a ? for each of the Args, which are values and not lists
	Where string
	Args  []any
	Group string
	Order string
	// Offset skips the first rows; 0 skips none
	Offset int
	// Limit is the most rows; a negative limit sets none
	Limit int
}

// SQL is the text of the select, with the placeholders of PostgreSQL
func (s *Select) SQL() string {
	var b strings.Builder
	b.Grow(64 + len(s.Columns) + len(s.Where) + len(s.Order))
	b.WriteString("SELECT ")
	if s.Columns == "" {
		b.WriteByte('*')
	} else {
		b.WriteString(s.Columns)
	}
	b.WriteString(" FROM ")
	writeTable(&b, s.Table)
	if s.Where != "" {
		b.WriteString(" WHERE ")
		n := 0
		for i := 0; i < len(s.Where); i++ {
			if c := s.Where[i]; c != '?' || n == len(s.Args) {
				b.WriteByte(c)
				continue
			}
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		}
	}
	if s.Group != "" {
		b.WriteString(" GROUP BY ")
		// a single name is quoted, an expression is not
		if len(strings.FieldsFunc(s.Group, utils.IsValidDBNameChar)) == 1 {
			DBConn.Dialector.QuoteTo(&b, s.Group)
		} else {
			b.WriteString(s.Group)
		}
	}
	if s.Order != "" {
		b.WriteString(" ORDER BY ")
		b.WriteString(s.Order)
	}
	if s.Limit >= 0 {
		b.WriteString(" LIMIT ")
		b.WriteString(strconv.Itoa(s.Limit))
	}
	if s.Offset > 0 {
		b.WriteString(" OFFSET ")
		b.WriteString(strconv.Itoa(s.Offset))
	}
	return b.String()
}

// writeTable writes the name of a table as gorm does: a name with a space as it is, the names of
// a schema and a table each quoted, any other name quoted
func writeTable(b *strings.Builder, name string) {
	if strings.ContainsAny(name, " `") {
		b.WriteString(name)
		return
	}
	DBConn.Dialector.QuoteTo(b, name)
}

// Query runs a select in the transaction, or out of any if tr is nil, and counts it as gorm counts
// its statements
func (tr *DbTransaction) Query(s *Select) (*sql.Rows, error) {
	db := GetDB(tr)
	text := s.SQL()
	var rows *sql.Rows
	err := db.Error
	if err == nil {
		rows, err = db.Statement.ConnPool.QueryContext(db.Statement.Context, text, s.Args...)
	}
	rowStatements.Inc()
	followStatement(db.Statement.Context, text)
	return rows, err
}
