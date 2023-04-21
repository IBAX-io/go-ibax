/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
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
	ID         uint32          `json:"id"`
	StateID    uint32          `json:"state"`
	TableID    string          `json:"tableid"`
	WalletID   string          `json:"walletid"`
	TokenID    string          `json:"tokenid"`
	Address    string          `json:"address"`
	Fields     []contractField `json:"fields"`
	Name       string          `json:"name"`
	AppId      uint32          `json:"app_id"`
	Ecosystem  uint32          `json:"ecosystem"`
	Conditions string          `json:"conditions"`
}

func getContractInfoHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	logger := getLogger(r)

	contract := getContract(r, params["name"])
	if contract == nil {
		logger.WithFields(log.Fields{"type": consts.ContractError, "contract_name": params["name"]}).Debug("contract name")
		errorResponse(w, errContract.Errorf(params["name"]))
		return
	}

	var result getContractResult
	info := getContractInfo(contract)
	con := &sqldb.Contract{}
	exits, err := con.Get(info.Owner.TableID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "contract_id": info.Owner.TableID}).Error("get contract")
		errorResponse(w, errQuery)
		return
	}
	if !exits {
		logger.WithFields(log.Fields{"type": consts.ContractError, "contract id": info.Owner.TableID}).Debug("get contract")
		errorResponse(w, errContract.Errorf(params["name"]))
		return
	}
	fields := make([]contractField, 0)
	result = getContractResult{
		ID:         uint32(info.Owner.TableID + consts.ShiftContractID),
		TableID:    converter.Int64ToStr(info.Owner.TableID),
		Name:       info.Name,
		StateID:    info.Owner.StateID,
		WalletID:   converter.Int64ToStr(info.Owner.WalletID),
		TokenID:    converter.Int64ToStr(info.Owner.TokenID),
		Address:    converter.AddressToString(info.Owner.WalletID),
		Ecosystem:  uint32(con.EcosystemID),
		AppId:      uint32(con.AppID),
		Conditions: con.Conditions,
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
