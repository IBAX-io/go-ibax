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

func (m Mode) GetAppParamHandler(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)

	form := &ecosystemForm{
		Validator: m.EcosystemGetter,
	}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	params := mux.Vars(r)

	ap := &sqldb.AppParam{}
	ap.SetTablePrefix(form.EcosystemPrefix)
	name := params["name"]
	found, err := ap.Get(nil, converter.StrToInt64(params["appID"]), name)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting app parameter by name")
		errorResponse(w, err)
		return
	}
	if !found {
		logger.WithFields(log.Fields{"type": consts.NotFound, "key": name}).Debug("app parameter not found")
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
