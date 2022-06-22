/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	log "github.com/sirupsen/logrus"
)

func getPlatformParamsHandler(w http.ResponseWriter, r *http.Request) {
	form := &paramsForm{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	logger := getLogger(r)

	list, err := sqldb.GetAllPlatformParameters(nil)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting all platform parameters")
	}

	result := &paramsResult{
		List: make([]paramResult, 0),
	}

	acceptNames := form.AcceptNames()
	for _, item := range list {
		if len(acceptNames) > 0 && !acceptNames[item.Name] {
			continue
		}
		result.List = append(result.List, paramResult{
			Name:       item.Name,
			Value:      item.Value,
			Conditions: item.Conditions,
		})
	}

	if len(result.List) == 0 {
		errorResponse(w, errParamNotFound.Errorf(form.Names), http.StatusBadRequest)
		return
	}

	jsonResponse(w, result)
}
