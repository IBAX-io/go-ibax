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

type myBalanceResult struct {
	Amount string `json:"amount"`
	Money  string `json:"money"`
}

func (m Mode) getMyBalanceHandler(w http.ResponseWriter, r *http.Request) {
	client := getClient(r)
	logger := getLogger(r)
	form := &ecosystemForm{
		Validator: m.EcosystemGetter,
	}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	keyID := client.KeyID
	if keyID == 0 {
		logger.WithFields(log.Fields{"type": consts.ConversionError, "value": converter.Int64ToStr(keyID)}).Error("converting wallet to address")
		errorResponse(w, errInvalidWallet.Errorf(converter.Int64ToStr(keyID)))
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

	jsonResponse(w, &myBalanceResult{
		Amount: key.Amount,
		Money:  converter.ChainMoney(key.Amount),
	})
}
