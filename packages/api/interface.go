/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/model"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type componentModel interface {
	SetTablePrefix(prefix string)
	Get(name string) (bool, error)
}

func getPageRowHandler(w http.ResponseWriter, r *http.Request) {
	getInterfaceRow(w, r, &model.Page{})
}

		errorResponse(w, errQuery)
		return
	} else if !ok {
		errorResponse(w, errNotFound)
		return
	}

	jsonResponse(w, c)
}
