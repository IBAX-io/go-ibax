/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

type paramResult struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Value      string `json:"value"`
	Conditions string `json:"conditions"`
}

type ecosystemParamsResult struct {
	List []paramResult `json:"list"`
}

func (m Mode) getEcosystemParamsHandler(w http.ResponseWriter, r *http.Request) {
	form := &appParamsForm{
		ecosystemForm: ecosystemForm{
			Validator: m.EcosystemGetter,
		},
	}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	logger := getLogger(r)

	sp := &sqldb.StateParameter{}
	sp.SetTablePrefix(form.EcosystemPrefix)
	list, err := sp.GetAllStateParameters()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting all state parameters")
	}

	result := &ecosystemParamsResult{
		List: make([]paramResult, 0),
	}

	acceptNames := form.AcceptNames()
	for _, item := range list {
		if len(acceptNames) > 0 && !acceptNames[item.Name] {
			continue
		}
		result.List = append(result.List, paramResult{
			ID:         converter.Int64ToStr(item.ID),
			Name:       item.Name,
			Value:      item.Value,
			Conditions: item.Conditions,
		})
	}

	jsonResponse(w, result)
}
