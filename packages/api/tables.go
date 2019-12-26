/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)

type tableInfo struct {
	Name  string `json:"name"`
	Count string `json:"count"`
}

type tablesResult struct {
	Count int64       `json:"count"`
	List  []tableInfo `json:"list"`
}

func getTablesHandler(w http.ResponseWriter, r *http.Request) {
	form := &paginatorForm{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadGateway)
		return
	}

	client := getClient(r)
	logger := getLogger(r)
	prefix := client.Prefix()

	table := &model.Table{}
	table.SetTablePrefix(prefix)

	count, err := table.Count()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("selecting records count from tables")
		errorResponse(w, err)
		return
	}

		List:  make([]tableInfo, len(list)),
	}
	for i, item := range list {
		err = model.GetTableQuery(item["name"], client.EcosystemID).Count(&count).Error
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("selecting count from table")
			errorResponse(w, err)
			return
		}

		result.List[i].Name = item["name"]
		result.List[i].Count = converter.Int64ToStr(count)
	}

	jsonResponse(w, result)
}
