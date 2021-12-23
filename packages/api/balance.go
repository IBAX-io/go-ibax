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

type balanceResult struct {
	Amount string `json:"amount"`
	Money  string `json:"money"`
}

func (m Mode) getBalanceHandler(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	form := &ecosystemForm{
		Validator: m.EcosystemGetter,
	}

	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	params := mux.Vars(r)

	keyID := converter.StringToAddress(params["wallet"])
	if keyID == 0 {
		logger.WithFields(log.Fields{"type": consts.ConversionError, "value": params["wallet"]}).Error("converting wallet to address")
		errorResponse(w, errInvalidWallet.Errorf(params["wallet"]))
		return
	}

	key := &sqldb.Key{}
	key.SetTablePrefix(form.EcosystemID)
	_, err := key.Get(nil, keyID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting Key for wallet")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, &balanceResult{
		Amount: key.Amount,
		Money:  converter.ChainMoney(key.Amount),
	})
}
