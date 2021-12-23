/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type rowResult struct {
	Value map[string]string `json:"value"`
}

type rowForm struct {
	Columns string `schema:"columns"`
}

func (f *rowForm) Validate(r *http.Request) error {
	if len(f.Columns) > 0 {
		f.Columns = converter.EscapeName(f.Columns)
	}
	return nil
}

func getRowHandler(w http.ResponseWriter, r *http.Request) {
	form := &rowForm{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	params := mux.Vars(r)
	client := getClient(r)
	logger := getLogger(r)

	q := sqldb.GetDB(nil).Limit(1)

	var (
		err   error
		table string
	)
	table, form.Columns, err = checkAccess(params["name"], form.Columns, client)
	if err != nil {
		errorResponse(w, err)
		return
	}
	col := `id`
	if len(params["column"]) > 0 {
		col = converter.Sanitize(params["column"], `-`)
	}
	if converter.FirstEcosystemTables[params["name"]] {
		q = q.Table(table).Where(col+" = ? and ecosystem = ?", params["id"], client.EcosystemID)
	} else {
		q = q.Table(table).Where(col+" = ?", params["id"])
	}

	if len(form.Columns) > 0 {
		q = q.Select(form.Columns)
	}

	rows, err := q.Rows()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).Error("Getting rows from table")
		errorResponse(w, errQuery)
		return
	}

	result, err := sqldb.GetResult(rows)
	if err != nil {
		errorResponse(w, err)
		return
	}

	if len(result) == 0 {
		errorResponse(w, errNotFound)
		return
	}

	jsonResponse(w, &rowResult{
		Value: result[0],
	})
}
