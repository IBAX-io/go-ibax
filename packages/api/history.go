/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/json"
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
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

	table := client.Prefix() + "_" + params["name"]
	rollbackTx := &sqldb.RollbackTx{}
	txs, err := rollbackTx.GetRollbackTxsByTableIDAndTableName(params["id"], table, rollbackHistoryLimit)
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
		rollbackList = append(rollbackList, rollback)
	}

	jsonResponse(w, &historyResult{rollbackList})
}
