/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package dataquery

import "net/http"

// Error is a refusal of a read, as the API answers it: {"error": Code, "msg": Msg} with Status
type Error struct {
	Code   string `json:"error"`
	Msg    string `json:"msg"`
	Status int    `json:"-"`
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Msg
}

// ErrAccessDenied refuses a table, or a column of it, whose read condition does not hold for the
// reader; msg is "<table>" or "<table>.<column>"
func ErrAccessDenied(what string) *Error {
	return &Error{Code: "E_ACCESS_DENIED", Msg: what, Status: http.StatusForbidden}
}

// ErrColumn refuses a column the table does not have
func ErrColumn(column string) *Error {
	return &Error{Code: "E_COLUMN", Msg: column, Status: http.StatusBadRequest}
}

// ErrValue refuses a value that is not one of its column's type
func ErrValue(column string) *Error {
	return &Error{Code: "E_VALUE", Msg: column, Status: http.StatusBadRequest}
}

// ErrWhere refuses a filter or an order that is not of the query language
func ErrWhere(msg string) *Error {
	return &Error{Code: "E_WHERE", Msg: msg, Status: http.StatusBadRequest}
}

// ErrLimit refuses a limit or an offset out of range
func ErrLimit(msg string) *Error {
	return &Error{Code: "E_LIMIT", Msg: msg, Status: http.StatusBadRequest}
}

// ErrTableNotFound refuses a table that does not exist
func ErrTableNotFound(table string) *Error {
	return &Error{Code: "E_TABLENOTFOUND", Msg: table, Status: http.StatusNotFound}
}

// ErrNotFound refuses a row that does not exist
func ErrNotFound(what string) *Error {
	return &Error{Code: "E_NOTFOUND", Msg: what, Status: http.StatusNotFound}
}

// ErrQuery is any failure of the database: its text stays in the node's log
var ErrQuery = &Error{Code: "E_QUERY", Msg: "DB query is wrong", Status: http.StatusInternalServerError}
