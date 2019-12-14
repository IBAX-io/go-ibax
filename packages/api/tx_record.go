/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/model"

	"github.com/gorilla/mux"
)

func getTxRecord(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
		}
	}
	jsonResponse(w, &resultList)
	return
}
