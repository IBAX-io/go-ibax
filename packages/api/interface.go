/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type componentModel interface {
	SetTablePrefix(prefix string)
	Get(name string) (bool, error)
}

func getPageRowHandler(w http.ResponseWriter, r *http.Request) {
	getInterfaceRow(w, r, &sqldb.Page{})
}

func getMenuRowHandler(w http.ResponseWriter, r *http.Request) {
	getInterfaceRow(w, r, &sqldb.Menu{})
}

func getSnippetRowHandler(w http.ResponseWriter, r *http.Request) {
	getInterfaceRow(w, r, &sqldb.Snippet{})
}

func getInterfaceRow(w http.ResponseWriter, r *http.Request, c componentModel) {
	params := mux.Vars(r)
	logger := getLogger(r)
	client := getClient(r)

	c.SetTablePrefix(client.Prefix())
	if ok, err := c.Get(params["name"]); err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting one row")
		errorResponse(w, errQuery)
		return
	} else if !ok {
		errorResponse(w, errNotFound)
		return
	}

	jsonResponse(w, c)
}
