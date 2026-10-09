/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/dataquery"

	"github.com/gorilla/mux"
)

type interfaceListResult struct {
	List []map[string]any `json:"list"`
}

// GET interface/{kind}/{name}: a page, menu or snippet as stored, and the hash of its source. Its
// ETag is the hash: a reader that holds the source asks with If-None-Match, and is answered 304
// while it is current.
func getInterfaceHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	ecosystem, err := queryEcosystem(r)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	element, err := dataquery.Element(dataReader(getClient(r)), params["kind"], params["name"], ecosystem)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	etag := `"` + element["hash"].(string) + `"`
	w.Header().Set("ETag", etag)
	for _, held := range strings.Split(r.Header.Get("If-None-Match"), ",") {
		if strings.TrimSpace(held) == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	jsonResponse(w, element)
}

// GET interface/{kind}?names=a,b: the elements of the names, at most 100; those that do not exist
// are left out
func getInterfacesHandler(w http.ResponseWriter, r *http.Request) {
	ecosystem, err := queryEcosystem(r)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	var names []string
	if text := r.URL.Query().Get("names"); text != "" {
		names = strings.Split(text, ",")
	}
	list, err := dataquery.Elements(dataReader(getClient(r)), mux.Vars(r)["kind"], names, ecosystem)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	jsonResponse(w, &interfaceListResult{List: list})
}
