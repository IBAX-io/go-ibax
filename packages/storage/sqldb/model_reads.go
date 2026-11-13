/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"database/sql"
	"errors"
	"fmt"
)

// The reads of the models of a transaction with a view of the state run in the view without
// database/sql and gorm: the statement gorm's First makes, of a fixed text, counted as gorm counts
// its queries, its row set into the model as gorm sets it.

// firstParamSQL and firstEcosystemSQL are the texts gorm's First makes of the reads of a parameter
// of an ecosystem by its name and of an ecosystem by its id, into a model without an id; the
// ByID texts those into a model with an id, which gorm adds to the condition
const (
	firstParamSQL         = `SELECT * FROM "1_parameters" WHERE ecosystem = $1 and name = $2 ORDER BY "1_parameters"."id" LIMIT 1`
	firstParamByIDSQL     = `SELECT * FROM "1_parameters" WHERE (ecosystem = $1 and name = $2) AND "1_parameters"."id" = $3 ORDER BY "1_parameters"."id" LIMIT 1`
	firstEcosystemSQL     = `SELECT * FROM "1_ecosystems" WHERE id = $1 ORDER BY "1_ecosystems"."id" LIMIT 1`
	firstEcosystemByIDSQL = `SELECT * FROM "1_ecosystems" WHERE id = $1 AND "1_ecosystems"."id" = $2 ORDER BY "1_ecosystems"."id" LIMIT 1`
)

// modelStatement is the statement of the read of a model of the text, counted as gorm's query
func modelStatement(text string) *Statement {
	s := NewStatement(text)
	s.metric = queryStatements
	return s
}

var (
	firstKeyStmt           = modelStatement(firstKeySQL)
	firstKeyByIDStmt       = modelStatement(firstKeyByIDSQL)
	keyStateStmt           = modelStatement(keyStateSQL)
	firstParamStmt         = modelStatement(firstParamSQL)
	firstParamByIDStmt     = modelStatement(firstParamByIDSQL)
	firstEcosystemStmt     = modelStatement(firstEcosystemSQL)
	firstEcosystemByIDStmt = modelStatement(firstEcosystemByIDSQL)
)

// keyStatement is the statement of the text of a read of a key
func keyStatement(text string) *Statement {
	switch text {
	case firstKeySQL:
		return firstKeyStmt
	case firstKeyByIDSQL:
		return firstKeyByIDStmt
	}
	return keyStateStmt
}

// rowScanner is what a read of a model reads of rows: those of database/sql or of a view
type rowScanner interface {
	Columns() ([]string, error)
	Next() bool
	Err() error
	Scan(dest ...any) error
}

// isFoundErr is isFound of the error of a read
func isFoundErr(err error) (bool, error) {
	if errors.Is(err, ErrRecordNotFound) {
		return false, nil
	}
	return true, err
}

// firstModel reads the first row of the statement s with the values args into a model in the view
// of the transaction, as gorm's First reads it there: field is the field of the model a column
// sets, a *int64, *string or *bool, or nil for a column the model has not; a column that is NULL
// leaves its field as it is. It tells false for no row, and ran is false when the statement did
// not run in a view, which leaves the read to gorm. A row that fails to scan sets the fields of the
// columns before the column that failed, as database/sql scans them.
func (tr *DbTransaction) firstModel(s *Statement, args []any, field func(column string) any) (found, ran bool, err error) {
	drv, ran, err := tr.directRun(s, s.Text, args, true)
	if !ran {
		return false, false, nil
	}
	if err != nil {
		return true, true, err
	}
	found, err = readModel(driverRows(drv), field)
	return found, true, err
}

// readModel reads the first row of rows into a model as gorm's First reads it and closes rows
func readModel(rows *Rows, field func(column string) any) (bool, error) {
	err := scanModel(rows, field)
	// gorm closes the rows last, adding the error of the close
	if cerr := rows.Close(); cerr != nil {
		err = addError(err, cerr)
	}
	return isFoundErr(err)
}

// scanModel scans the first row of rows into the fields of a model as gorm's Scan scans a row into
// a struct, its error the one gorm adds of the scan, of the rows and of no row
func scanModel(rows rowScanner, field func(column string) any) error {
	var err error
	affected := false
	columns, _ := rows.Columns()
	if rows.Next() {
		dest := make([]any, len(columns))
		for i, c := range columns {
			switch f := field(c).(type) {
			case *int64:
				dest[i] = new(*int64)
			case *string:
				dest[i] = new(*string)
			case *bool:
				dest[i] = new(*bool)
			case nil:
				dest[i] = new(sql.RawBytes)
			default:
				panic(fmt.Sprintf("field %T of column %s", f, c))
			}
		}
		affected = true
		err = addError(err, rows.Scan(dest...))
		for i, c := range columns {
			switch f := field(c).(type) {
			case *int64:
				if p := *dest[i].(**int64); p != nil {
					*f = *p
				}
			case *string:
				if p := *dest[i].(**string); p != nil {
					*f = *p
				}
			case *bool:
				if p := *dest[i].(**bool); p != nil {
					*f = *p
				}
			}
		}
	}
	if rerr := rows.Err(); rerr != nil && rerr != err {
		err = addError(err, rerr)
	}
	if !affected && err == nil {
		err = ErrRecordNotFound
	}
	return err
}

// addError is gorm's AddError: the first error, or the errors joined
func addError(err, add error) error {
	if add == nil {
		return err
	}
	if err == nil {
		return add
	}
	return fmt.Errorf("%v; %w", err, add)
}

// field is the field of the parameter a column of 1_parameters sets, as gorm's schema names them
func (sp *StateParameter) field(column string) any {
	switch column {
	case "id":
		return &sp.ID
	case "name":
		return &sp.Name
	case "value":
		return &sp.Value
	case "conditions":
		return &sp.Conditions
	}
	return nil
}

// field is the field of the ecosystem a column of 1_ecosystems sets, as gorm's schema names them
func (sys *Ecosystem) field(column string) any {
	switch column {
	case "id":
		return &sys.ID
	case "name":
		return &sys.Name
	case "is_valued":
		return &sys.IsValued
	case "emission_amount":
		return &sys.EmissionAmount
	case "token_symbol":
		return &sys.TokenSymbol
	case "token_name":
		return &sys.TokenName
	case "type_emission":
		return &sys.TypeEmission
	case "type_withdraw":
		return &sys.TypeWithdraw
	case "digits":
		return &sys.Digits
	case "info":
		return &sys.Info
	case "fee_mode_info":
		return &sys.FeeModeInfo
	case "frozen":
		return &sys.Frozen
	}
	return nil
}

// firstByName reads the parameter name of the ecosystem of sp into sp as gorm's First reads it,
// with the id of sp a condition when it has one
func (sp *StateParameter) firstByName(dbTx *DbTransaction, name string) (bool, error) {
	s, args := firstParamStmt, []any{sp.ecosystem, name}
	if sp.ID != 0 {
		s, args = firstParamByIDStmt, append(args, sp.ID)
	}
	if found, ran, err := dbTx.firstModel(s, args, sp.field); ran {
		return found, err
	}
	return isFound(GetDB(dbTx).Where("ecosystem = ? and name = ?", sp.ecosystem, name).First(sp))
}

// firstByID reads the ecosystem id into sys as gorm's First reads it, with the id of sys a
// condition when it has one
func (sys *Ecosystem) firstByID(dbTx *DbTransaction, id int64) (bool, error) {
	s, args := firstEcosystemStmt, []any{id}
	if sys.ID != 0 {
		s, args = firstEcosystemByIDStmt, append(args, sys.ID)
	}
	if found, ran, err := dbTx.firstModel(s, args, sys.field); ran {
		return found, err
	}
	return isFound(GetDB(dbTx).First(sys, "id = ?", id))
}
