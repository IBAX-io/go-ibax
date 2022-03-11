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

func getEcosystemNameHandler(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)

	ecosystemID := converter.StrToInt64(r.FormValue("id"))
	ecosystems := sqldb.Ecosystem{}
	found, err := ecosystems.Get(nil, ecosystemID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on getting ecosystem name")
		errorResponse(w, err)
		return
	}
	if !found {
		logger.WithFields(log.Fields{"type": consts.NotFound, "ecosystem_id": ecosystemID}).Debug("ecosystem by id not found")
		errorResponse(w, errParamNotFound.Errorf("name"))
		return
	}

	jsonResponse(w, &struct {
		EcosystemName string `json:"ecosystem_name"`
	}{
		EcosystemName: ecosystems.Name,
	})
}
