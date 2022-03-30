/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/gorilla/mux"
)

func getTxRecord(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	hashes := params["hashes"]

	var (
		hashList   []string
		resultList []any
	)
	if len(hashes) > 0 {
		hashList = strings.Split(hashes, ",")
	}
	for _, hashStr := range hashList {

		if result, err := sqldb.GetTxRecord(nil, hashStr); err == nil {
			resultList = append(resultList, result)
		}
	}
	jsonResponse(w, &resultList)
	return
}
