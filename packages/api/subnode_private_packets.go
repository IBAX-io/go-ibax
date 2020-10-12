/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)
	if err != nil {
		logger.WithFields(log.Fields{"error": err}).Error("Error reading private data list")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
