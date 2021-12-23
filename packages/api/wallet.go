/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/hex"
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/shopspring/decimal"
)

// History represent record of history table
type WalletHistory struct {
	tableName    string
	ID           int64
	SenderID     int64
	SenderAdd    string
	RecipientID  int64
	RecipientAdd string
	Amount       decimal.Decimal
	Comment      string
	BlockID      int64
	TxHash       string
	CreatedAt    int64
	Money        string
}

type walletHistoryForm struct {
	Limit      int    `schema:"limit"`
	Page       int    `schema:"page"`
	SearchType string `schema:"searchType"`
}

func (f *walletHistoryForm) Validate(r *http.Request) error {
	if len(f.SearchType) == 0 {
		f.SearchType = ""
	}
	return nil
}
func getWalletHistory(w http.ResponseWriter, r *http.Request) {
	form := &walletHistoryForm{}
	token := getToken(r)
	var err error

	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	if form.Limit == 0 {
		form.Limit = 20
	}
	if form.Page == 0 {
		form.Page = 1
	}
	form.Page = (form.Page - 1) * form.Limit

	if token != nil && token.Valid {
		if claims, ok := token.Claims.(*JWTClaims); ok {
			keyId := claims.KeyID
			var (
				histories       []sqldb.History
				walletHistories []WalletHistory
			)
			histories, err = sqldb.GetWalletRecordHistory(nil, keyId, form.SearchType, form.Limit, form.Page)
			if err == nil {
				if len(histories) > 0 {
					for _, history := range histories {
						var walletHistory WalletHistory
						walletHistory.Amount = history.Amount
						walletHistory.Money = converter.ChainMoney(history.Amount.String())
						walletHistory.BlockID = history.BlockID
						walletHistory.SenderID = history.SenderID
						walletHistory.RecipientID = history.RecipientID
						walletHistory.TxHash = hex.EncodeToString(history.TxHash)
						walletHistory.Comment = history.Comment
						walletHistory.CreatedAt = history.CreatedAt
						walletHistory.ID = history.ID
						walletHistory.SenderAdd = smart.IDToAddress(history.SenderID)
						walletHistory.RecipientAdd = smart.IDToAddress(history.RecipientID)
						walletHistories = append(walletHistories, walletHistory)
					}
					jsonResponse(w, walletHistories)
				} else {
					jsonResponse(w, make([]string, 0, 0))
				}
			}
		}
		errorResponse(w, err, http.StatusBadRequest)
		return
	} else {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
}
