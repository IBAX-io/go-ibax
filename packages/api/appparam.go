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

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func (m Mode) GetAppParamHandler(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)

	form := &ecosystemForm{
		Validator: m.EcosysIDValidator,
	}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	params := mux.Vars(r)

	ap := &model.AppParam{}
	ap.SetTablePrefix(form.EcosystemPrefix)
	name := params["name"]
	if !found {
		logger.WithFields(log.Fields{"type": consts.NotFound, "key": name}).Error("app parameter not found")
		errorResponse(w, errParamNotFound.Errorf(name))
		return
	}

	jsonResponse(w, &paramResult{
		ID:         converter.Int64ToStr(ap.ID),
		Name:       ap.Name,
		Value:      ap.Value,
		Conditions: ap.Conditions,
	})
}
