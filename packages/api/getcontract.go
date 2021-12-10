/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/script"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type contractField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Optional bool   `json:"optional"`
}

type getContractResult struct {
	ID       uint32          `json:"id"`
	StateID  uint32          `json:"state"`
	TableID  string          `json:"tableid"`
	WalletID string          `json:"walletid"`
	TokenID  string          `json:"tokenid"`
	Address  string          `json:"address"`
	Fields   []contractField `json:"fields"`
	Name     string          `json:"name"`
}

func getContractInfoHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	contract := getContract(r, params["name"])
	if contract == nil {
		logger.WithFields(log.Fields{"type": consts.ContractError, "contract_name": params["contract"]}).Debug("contract name")
		errorResponse(w, errContract.Errorf(params["name"]))
		return
	}

	var result getContractResult
	info := getContractInfo(contract)
	fields := make([]contractField, 0)
	result = getContractResult{
		ID:       uint32(info.Owner.TableID + consts.ShiftContractID),
		TableID:  converter.Int64ToStr(info.Owner.TableID),
		Name:     info.Name,
		StateID:  info.Owner.StateID,
		WalletID: converter.Int64ToStr(info.Owner.WalletID),
		TokenID:  converter.Int64ToStr(info.Owner.TokenID),
		Address:  converter.AddressToString(info.Owner.WalletID),
	}

	if info.Tx != nil {
		for _, fitem := range *info.Tx {
			fields = append(fields, contractField{
				Name:     fitem.Name,
				Type:     script.OriginalToString(fitem.Original),
				Optional: fitem.ContainsTag(script.TagOptional),
			})
		}
	}
	result.Fields = fields

	jsonResponse(w, result)
}
