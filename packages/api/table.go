/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/dataquery"

	"github.com/gorilla/mux"
)

type tableResult struct {
	Name       string             `json:"name"`
	Insert     string             `json:"insert"`
	NewColumn  string             `json:"new_column"`
	Update     string             `json:"update"`
	Read       string             `json:"read"`
	Filter     string             `json:"filter"`
	Rows       string             `json:"rows"`
	Conditions string             `json:"conditions"`
	AppID      string             `json:"app_id"`
	Columns    []dataquery.Column `json:"columns"`
}

// GET table/{name}: the table's permissions and its columns with their types. A definition is
// public: reading the rows is what the read conditions guard.
func getTableHandler(w http.ResponseWriter, r *http.Request) {
	ecosystem, err := queryEcosystem(r)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	t, err := dataquery.Describe(dataReader(getClient(r)), mux.Vars(r)["name"], ecosystem)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	perm := t.Record.Permissions
	jsonResponse(w, &tableResult{
		Name:       t.Name,
		Insert:     perm.Insert,
		NewColumn:  perm.NewColumn,
		Update:     perm.Update,
		Read:       perm.Read,
		Filter:     perm.Filter,
		Rows:       perm.Rows,
		Conditions: t.Record.Conditions,
		AppID:      converter.Int64ToStr(t.Record.AppID),
		Columns:    t.Columns(),
	})
}
