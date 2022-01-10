/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type columnInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Perm string `json:"perm"`
}

type tableResult struct {
	Name       string       `json:"name"`
	Insert     string       `json:"insert"`
	NewColumn  string       `json:"new_column"`
	Update     string       `json:"update"`
	Read       string       `json:"read,omitempty"`
	Filter     string       `json:"filter,omitempty"`
	Conditions string       `json:"conditions"`
	AppID      string       `json:"app_id"`
	Columns    []columnInfo `json:"columns"`
}

func getTableHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)
	client := getClient(r)
	prefix := client.Prefix()

	table := &sqldb.Table{}
	table.SetTablePrefix(prefix)

	_, err := table.Get(nil, strings.ToLower(params["name"]))
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting table")
		errorResponse(w, err)
		return
	}

	if len(table.Name) == 0 {
		errorResponse(w, errTableNotFound.Errorf(params["name"]))
		return
	}

	var columnsMap map[string]string
	err = json.Unmarshal([]byte(table.Columns), &columnsMap)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("Unmarshalling table columns to json")
		errorResponse(w, err)
		return
	}

	columns := make([]columnInfo, 0)
	for key, value := range columnsMap {
		colType, err := sqldb.NewDbTransaction(nil).GetColumnType(prefix+`_`+params["name"], key)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting column type from db")
			errorResponse(w, err)
			return
		}
		columns = append(columns, columnInfo{
			Name: key,
			Perm: value,
			Type: colType,
		})
	}

	jsonResponse(w, &tableResult{
		Name:       table.Name,
		Insert:     table.Permissions.Insert,
		NewColumn:  table.Permissions.NewColumn,
		Update:     table.Permissions.Update,
		Read:       table.Permissions.Read,
		Filter:     table.Permissions.Filter,
		Conditions: table.Conditions,
		AppID:      converter.Int64ToStr(table.AppID),
		Columns:    columns,
	})
}
