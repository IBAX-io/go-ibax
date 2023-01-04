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
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

type balanceResult struct {
	Amount      string `json:"amount"`
	Digits      int64  `json:"digits"`
	Total       string `json:"total"`
	Utxo        string `json:"utxo"`
	TokenSymbol string `json:"token_symbol"`
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
	accountAmount, _ := decimal.NewFromString(key.Amount)

	sp := &sqldb.SpentInfo{}
	utxoAmount, err := sp.GetBalance(nil, keyID, form.EcosystemID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting UTXO Key for wallet")
		errorResponse(w, err)
		return
	}
	total := utxoAmount.Add(accountAmount)

	eco := sqldb.Ecosystem{}
	_, err = eco.Get(nil, form.EcosystemID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting key balance token symbol")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, &balanceResult{
		Amount:      key.Amount,
		Digits:      eco.Digits,
		Total:       total.String(),
		Utxo:        utxoAmount.String(),
		TokenSymbol: eco.TokenSymbol,
	})
}
