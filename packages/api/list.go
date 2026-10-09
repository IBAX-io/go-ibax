/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/dataquery"

	"github.com/gorilla/mux"
)

type listResult struct {
	Count int64               `json:"count"`
	List  []map[string]string `json:"list"`
}

type sumResult struct {
	Sum string `json:"sum"`
}

// The most a data request body holds
const maxQueryBody = 1 << 20

func dataReader(client *Client) dataquery.Reader {
	return dataquery.Reader{KeyID: client.KeyID, AccountID: client.AccountID, Ecosystem: client.EcosystemID}
}

func dataErrorResponse(w http.ResponseWriter, err error) {
	var refusal *dataquery.Error
	if errors.As(err, &refusal) {
		errorResponse(w, errType{Err: refusal.Code, Message: refusal.Msg, Status: refusal.Status})
		return
	}
	errorResponse(w, err)
}

// readBody reads a JSON request body into v: no other fields than v has, nothing after it
func readBody(r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return dataquery.ErrWhere("the body must be application/json")
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxQueryBody+1))
	if err != nil || len(data) > maxQueryBody {
		return dataquery.ErrWhere("the body is too large")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil || decoder.More() {
		return dataquery.ErrWhere("the body is not a query")
	}
	return nil
}

// queryEcosystem reads the ecosystem a GET names, 0 when none
func queryEcosystem(r *http.Request) (int64, error) {
	text := r.URL.Query().Get("ecosystem")
	if text == "" {
		return 0, nil
	}
	ecosystem, err := strconv.ParseInt(text, 10, 64)
	if err != nil || ecosystem <= 0 {
		return 0, dataquery.ErrWhere("ecosystem " + text)
	}
	return ecosystem, nil
}

// queryColumns reads the columns a GET names, comma separated
func queryColumns(r *http.Request) []string {
	text := r.URL.Query().Get("columns")
	if text == "" {
		return nil
	}
	return strings.Split(text, ",")
}

func runQuery(w http.ResponseWriter, r *http.Request, q dataquery.Query) {
	t, err := dataquery.Open(dataReader(getClient(r)), mux.Vars(r)["name"], q.Ecosystem)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	result, err := t.Query(q)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

// GET list/{name}: a query in the URL, its where and order as JSON
func getListHandler(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	q := dataquery.Query{Columns: queryColumns(r)}
	var err error
	if q.Ecosystem, err = queryEcosystem(r); err != nil {
		dataErrorResponse(w, err)
		return
	}
	if text := values.Get("where"); text != "" {
		if err := dataquery.DecodeJSON([]byte(text), &q.Where); err != nil {
			dataErrorResponse(w, err)
			return
		}
	}
	if text := values.Get("order"); text != "" {
		if err := dataquery.DecodeJSON([]byte(text), &q.Order); err != nil {
			dataErrorResponse(w, err)
			return
		}
	}
	for name, target := range map[string]*int{"limit": nil, "offset": &q.Offset} {
		text := values.Get(name)
		if text == "" {
			continue
		}
		n, err := strconv.Atoi(text)
		if err != nil {
			dataErrorResponse(w, dataquery.ErrLimit(name+" "+text))
			return
		}
		if target == nil {
			q.Limit = &n
		} else {
			*target = n
		}
	}
	runQuery(w, r, q)
}

// POST listWhere/{name}: a query as a JSON body
func getListWhereHandler(w http.ResponseWriter, r *http.Request) {
	var q dataquery.Query
	if err := readBody(r, &q); err != nil {
		dataErrorResponse(w, err)
		return
	}
	runQuery(w, r, q)
}

// POST sumWhere/{name}: {ecosystem, column, where} → the sum of the column over the rows matched
func getsumWhereHandler(w http.ResponseWriter, r *http.Request) {
	var form struct {
		Ecosystem int64           `json:"ecosystem"`
		Column    string          `json:"column"`
		Where     dataquery.Where `json:"where"`
	}
	if err := readBody(r, &form); err != nil {
		dataErrorResponse(w, err)
		return
	}
	t, err := dataquery.Open(dataReader(getClient(r)), mux.Vars(r)["name"], form.Ecosystem)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	sum, err := t.Sum(form.Column, form.Where)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	jsonResponse(w, &sumResult{Sum: sum})
}
