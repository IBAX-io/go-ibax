/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/dataquery"

	"github.com/gorilla/mux"
)

type rowResult = dataquery.RowResult

// GET row/{name}/{id} and row/{name}/{column}/{id}: the row whose id, or column, holds the value
func getRowHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	ecosystem, err := queryEcosystem(r)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	t, err := dataquery.Open(dataReader(getClient(r)), params["name"], ecosystem)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	result, err := t.Row(params["column"], params["id"], queryColumns(r))
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}
