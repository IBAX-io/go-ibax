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

func privateDataListHandlre(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	privateData := model.PrivatePackets{}

	result, err := privateData.GetAll()
