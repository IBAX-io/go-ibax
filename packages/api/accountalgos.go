/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"fmt"
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

// getAccountAlgorithmsHandler is account_algorithms with the number of keys of each algorithm
func getAccountAlgorithmsHandler(w http.ResponseWriter, r *http.Request) {
	list, err := syspar.AccountAlgorithmsWithKeys()
	if err != nil {
		getLogger(r).WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("counting the keys of account algorithms")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, list)
}

// getAccountAlgoKeysHandler is a page of the accounts whose keys have an algorithm
func getAccountAlgoKeysHandler(w http.ResponseWriter, r *http.Request) {
	form := &paginatorForm{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	if form.Offset < 0 {
		errorResponse(w, fmt.Errorf("offset %d is negative", form.Offset), http.StatusBadRequest)
		return
	}
	algo, err := syspar.ParseAccountAlgo(mux.Vars(r)["algo"])
	if err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	page, err := syspar.KeysOfAccountAlgo(algo, form.Offset, form.Limit)
	if err != nil {
		getLogger(r).WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("listing the keys of an account algorithm")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, page)
}
