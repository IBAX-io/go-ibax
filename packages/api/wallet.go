/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"encoding/hex"
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/shopspring/decimal"
)

// History represent record of history table
type walletHistory struct {
	ID           int64           `json:"id"`
	SenderID     int64           `json:"sender_id"`
	SenderAdd    string          `json:"sender_add"`
	RecipientID  int64           `json:"recipient_id"`
	RecipientAdd string          `json:"recipient_add"`
	Amount       decimal.Decimal `json:"amount"`
	Comment      string          `json:"comment"`
	BlockID      int64           `json:"block_id"`
	TxHash       string          `json:"tx_hash"`
	CreatedAt    int64           `json:"created_at"`
	Money        string          `json:"money"`
}

type WalletHistoryResponse struct {
	Page  int             `json:"page"`
	Limit int             `json:"limit"`
	Total int64           `json:"total"`
	List  []walletHistory `json:"list"`
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
	var (
		err  error
		rets WalletHistoryResponse
	)

	if err = parseForm(r, form); err != nil {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}

	if form.Limit <= 0 {
		form.Limit = 20
	} else if form.Limit > 500 {
		form.Limit = 500
	}
	if form.Page <= 0 {
		form.Page = 1
	}
	rets.Page = form.Page
	rets.Limit = form.Limit
	form.Page = (form.Page - 1) * form.Limit

	if token != nil && token.Valid {
		if claims, ok := token.Claims.(*JWTClaims); ok {
			keyId := claims.KeyID
			var (
				histories []sqldb.History
				total     int64
			)
			histories, total, err = sqldb.GetWalletRecordHistory(nil, keyId, form.SearchType, form.Limit, form.Page)
			if err == nil {
				rets.Total = total
				for _, history := range histories {
					var hy walletHistory
					hy.Amount = history.Amount
					hy.Money = converter.ChainMoney(history.Amount.String())
					hy.BlockID = history.BlockID
					hy.SenderID = history.SenderID
					hy.RecipientID = history.RecipientID
					hy.TxHash = hex.EncodeToString(history.TxHash)
					hy.Comment = history.Comment
					hy.CreatedAt = history.CreatedAt
					hy.ID = history.ID
					hy.SenderAdd = converter.IDToAddress(history.SenderID)
					hy.RecipientAdd = converter.IDToAddress(history.RecipientID)
					rets.List = append(rets.List, hy)
				}
				if rets.List == nil {
					rets.List = make([]walletHistory, 0)
				}
				jsonResponse(w, rets)
				return
			}
		}
		errorResponse(w, err, http.StatusBadRequest)
		return
	} else {
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
}
