/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/publisher"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type configOptionHandler func(w http.ResponseWriter, option string) error

func getConfigOptionHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	if len(params["option"]) == 0 {
		logger.WithFields(log.Fields{"type": consts.EmptyObject, "error": "option not specified"}).Error("on getting option in config handler")
		errorResponse(w, errNotFound)
		return
	}

	switch params["option"] {
	case "centrifugo":
		centrifugoAddressHandler(w, r)
		return
	}

	errorResponse(w, errNotFound)
}

// centrifugoAddressHandler answers where and how clients connect to Centrifugo, or 503 when it
// does not answer
func centrifugoAddressHandler(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)

	endpoint, err := publisher.GetEndpoint()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.CentrifugoError, "error": err}).Warn("on getting centrifugo info")
		errorResponse(w, errCentrifugo.Errorf(err))
		return
	}

	jsonResponse(w, endpoint)
}
