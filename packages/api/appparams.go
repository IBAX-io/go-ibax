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

	"github.com/gorilla/mux"

	log "github.com/sirupsen/logrus"
)

type appParamsResult struct {
	App  string        `json:"app_id"`
	List []paramResult `json:"list"`
}

type appParamsForm struct {
	ecosystemForm
	paramsForm
}

func (f *appParamsForm) Validate(r *http.Request) error {
	return f.ecosystemForm.Validate(r)
}

func (m Mode) getAppParamsHandler(w http.ResponseWriter, r *http.Request) {
	form := &appParamsForm{
		ecosystemForm: ecosystemForm{
			Validator: m.EcosystemGetter,
		},
	}

	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	params := mux.Vars(r)
	logger := getLogger(r)

	ap := &sqldb.AppParam{}
	ap.SetTablePrefix(form.EcosystemPrefix)

	list, err := ap.GetAllAppParameters(converter.StrToInt64(params["appID"]))
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting all app parameters")
	}

	result := &appParamsResult{
		App:  params["appID"],
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
