/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)

func getSystemParamsHandler(w http.ResponseWriter, r *http.Request) {
	form := &paramsForm{}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	logger := getLogger(r)

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
