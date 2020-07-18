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

		if result, err := model.GetTxRecord(nil, hashStr); err == nil {
			resultList = append(resultList, result)
		}
	}
	jsonResponse(w, &resultList)
	return
}
