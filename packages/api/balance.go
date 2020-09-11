/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
type balanceResult struct {
	Amount string `json:"amount"`
	Money  string `json:"money"`
}

type myAssignBalanceResult struct {
	Show    bool   `json:"show"`
	Amount  string `json:"amount"`
	Balance string `json:"balance"`
}

func (m Mode) getBalanceHandler(w http.ResponseWriter, r *http.Request) {
	logger := getLogger(r)
	form := &ecosystemForm{
		Validator: m.EcosysIDValidator,
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

	key := &model.Key{}
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

func (m Mode) getMyAssignBalanceHandler(w http.ResponseWriter, r *http.Request) {
	client := getClient(r)
	logger := getLogger(r)
	ret := model.Response{}
	form := &ecosystemForm{
		Validator: m.EcosysIDValidator,
	}
	if err := parseForm(r, form); err != nil {
		//errorResponse(w, err, http.StatusBadRequest)
		ret.Return(nil, model.CodeRequestformat.Errorf(err))
		JsonCodeResponse(w, &ret)
		return
	}

	keyID := client.KeyID
	if keyID == 0 {
		logger.WithFields(log.Fields{"type": consts.ConversionError, "value": converter.Int64ToStr(keyID)}).Error("converting wallet to address")
		ret.Return(nil, model.CodeRequestformat.Errorf(errors.New(converter.Int64ToStr(keyID))))
		JsonCodeResponse(w, &ret)
		return
	}

	key := &model.AssignGetInfo{}
	fg, balance, total_balance, err := key.GetBalance(nil, keyID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting Key for wallet")
		ret.Return(nil, model.CodeDBfinderr.Errorf(err))
		JsonCodeResponse(w, &ret)
		return
	}

	ret.Return(myAssignBalanceResult{
		Show:    fg,
		Amount:  balance.String(),
		Balance: total_balance.String(),
	}, model.CodeSuccess)
	JsonCodeResponse(w, &ret)
}
