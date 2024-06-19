/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"encoding/json"
	"errors"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"net/http"
)

type accountsApi struct {
	Mode
}

func newAccountsApi(m Mode) *accountsApi {
	return &accountsApi{m}
}

func (c *accountsApi) GetKeysCount(ctx RequestContext) (*int64, *Error) {
	r := ctx.HTTPRequest()
	cnt, err := sqldb.GetKeysCount()
	if err != nil {
		logger := getLogger(r)
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on getting keys count")
		return nil, InternalError(err.Error())
	}

	return &cnt, nil
}

type BalanceResult struct {
	Amount      string `json:"amount"`
	Digits      int64  `json:"digits"`
	Total       string `json:"total"`
	Utxo        string `json:"utxo"`
	TokenSymbol string `json:"token_symbol"`
	TokenName   string `json:"token_name"`
}

type AccountOrKeyId struct {
	KeyId   int64  `json:"key_id,omitempty"`
	Account string `json:"account,omitempty"`
}

func (bh *AccountOrKeyId) Validate(r *http.Request) error {
	if bh == nil {
		return errors.New(paramsEmpty)
	}
	if bh.KeyId == 0 {
		return errors.New("invalid input")
	}

	return nil
}

// UnmarshalJSON verify input, keyId is preferred
func (bh *AccountOrKeyId) UnmarshalJSON(data []byte) error {
	type rename AccountOrKeyId
	info := rename{}
	err := json.Unmarshal(data, &info)
	if err == nil {
		if info.KeyId != 0 {
			if converter.IDToAddress(info.KeyId) == `invalid` {
				return errors.New("invalid key id")
			}
			bh.KeyId = info.KeyId
			return nil
		}
		keyId := converter.AddressToID(info.Account)
		if keyId == 0 {
			return errors.New("invalid Account")
		}
		bh.KeyId = keyId
		return nil
	}
	var input string
	err = json.Unmarshal(data, &input)
	if err != nil {
		return err
	}
	keyId := converter.AddressToID(input)
	if keyId == 0 {
		return errors.New("invalid key id or account address")
	}
	bh.KeyId = keyId
	return nil
}

func (b *accountsApi) GetBalance(ctx RequestContext, info *AccountOrKeyId, ecosystemId *int64) (*BalanceResult, *Error) {
	r := ctx.HTTPRequest()
	logger := getLogger(r)
	form := &ecosystemForm{
		Validator: b.EcosystemGetter,
	}
	if ecosystemId != nil {
		form.EcosystemID = *ecosystemId
	}

	if err := parameterValidator(r, form); err != nil {
		return nil, InvalidParamsError(err.Error())
	}
	if err := parameterValidator(r, info); err != nil {
		return nil, InvalidParamsError(err.Error())
	}
	keyId := info.KeyId

	key := &sqldb.Key{}
	key.SetTablePrefix(form.EcosystemID)
	_, err := key.Get(nil, keyId)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting Key for wallet")
		return nil, DefaultError(err.Error())
	}
	accountAmount, _ := decimal.NewFromString(key.Amount)

	sp := &sqldb.SpentInfo{}
	utxoAmount, err := sp.GetBalance(nil, keyId, form.EcosystemID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting UTXO Key for wallet")
		return nil, DefaultError(err.Error())
	}
	total := utxoAmount.Add(accountAmount)

	eco := sqldb.Ecosystem{}
	_, err = eco.Get(nil, form.EcosystemID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting key balance token symbol")
		return nil, DefaultError(err.Error())
	}

	return &BalanceResult{
		Amount:      key.Amount,
		Digits:      eco.Digits,
		Total:       total.String(),
		Utxo:        utxoAmount.String(),
		TokenSymbol: eco.TokenSymbol,
		TokenName:   eco.TokenName,
	}, nil
}
