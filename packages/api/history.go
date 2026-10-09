/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/json"
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/dataquery"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

const rollbackHistoryLimit = 100

type historyResult struct {
	List []map[string]string `json:"list"`
}

func getHistoryHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	client := getClient(r)

	ecosystem, err := queryEcosystem(r)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	t, err := dataquery.Open(dataReader(client), params["name"], ecosystem)
	if err != nil {
		dataErrorResponse(w, err)
		return
	}
	// The past values of a row the reader sees now
	if err := t.Sees(params["id"]); err != nil {
		dataErrorResponse(w, err)
		return
	}
	rollbackTx := &sqldb.RollbackTx{}
	txs, err := rollbackTx.GetRollbackTxsByTableIDAndTableName(params["id"], t.Physical(), rollbackHistoryLimit)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("rollback history")
		errorResponse(w, err)
		return
	}
	rollbackList := []map[string]string{}
	for _, tx := range *txs {
		if tx.Data == "" {
			continue
		}
		rollback := map[string]string{}
		if err := json.Unmarshal([]byte(tx.Data), &rollback); err != nil {
			logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling rollbackTx.Data from JSON")
			errorResponse(w, err)
			return
		}
		// A past value is read as the column it was a value of is
		for name := range rollback {
			if !t.Readable(name) {
				delete(rollback, name)
			}
		}
		rollbackList = append(rollbackList, rollback)
	}

	jsonResponse(w, &historyResult{rollbackList})
}
