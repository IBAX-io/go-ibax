/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/utxo"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type utxoMovementsForm struct {
	ecosystemForm
	Before int64 `schema:"before"`
	Limit  int   `schema:"limit"`
}

func (f *utxoMovementsForm) Validate(r *http.Request) error {
	if f.Limit == 0 {
		f.Limit = utxo.DefaultLimit
	}
	if f.Before < 0 || f.Limit < 1 || f.Limit > utxo.MaxLimit {
		return errUTXOPaging.Errorf(f.Before, f.Limit)
	}
	return f.ecosystemForm.Validate(r)
}

// The UTXO movements of an account, as public as its balance
func (m Mode) getUTXOMovementsHandler(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	form := &utxoMovementsForm{ecosystemForm: ecosystemForm{Validator: m.EcosystemGetter}}
	if err := parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	wallet := mux.Vars(r)["wallet"]
	keyID := converter.StringToAddress(wallet)
	if keyID == 0 {
		logger.WithFields(log.Fields{"type": consts.ConversionError, "value": wallet}).Error("converting wallet to address")
		errorResponse(w, errInvalidWallet.Errorf(wallet))
		return
	}
	page, err := utxo.Movements(keyID, form.EcosystemID, form.Before, form.Limit)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("listing UTXO movements")
		errorResponse(w, err)
		return
	}
	jsonResponse(w, page)
}
