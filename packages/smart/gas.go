package smart

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

type multiPays []struct {
	toID, fromID                          int64
	fuelRate, storageFuel, newElementFuel decimal.Decimal
	payWallet                             model.Key
	tokenEco                              int64
}

func (sc *SmartContract) payContract(errNeedPay bool) error {
	sc.Penalty = errNeedPay
	for _, pay := range sc.multiPays {
		placeholder := `taxes for execution of %s contract`
		comment := fmt.Sprintf(placeholder, sc.TxContract.Name)
		money := sc.TxUsedCost.Mul(pay.fuelRate).Add(pay.storageFuel)
		if len(sc.TxSmart.Expedite) > 0 {
			money = money.Add(StringToAmount(sc.TxSmart.Expedite))
		}
		if !errNeedPay {
			money = money.Add(pay.newElementFuel)
		} else {
			comment = "(error)" + comment
			ts := model.TransactionStatus{}
			if err := ts.UpdatePenalty(sc.DbTransaction, sc.TxHash); err != nil {
				return err
			}
		}

		wltAmount := pay.payWallet.CapableAmount()
		if wltAmount.Cmp(money) < 0 {
			return errTaxes
		}
		taxes := money.Mul(decimal.New(syspar.SysInt64(syspar.TaxesSize), 0)).Div(decimal.New(100, 0)).Floor()
		fromIDStr := converter.Int64ToStr(pay.fromID)
		if err := sc.payTaxes(converter.Int64ToStr(pay.toID), money.Sub(taxes), 1, pay.tokenEco, fromIDStr, comment); err != nil {
			return err
		}

		if err := sc.payTaxes(syspar.GetTaxesWallet(pay.tokenEco), taxes, 2, pay.tokenEco, fromIDStr, comment); err != nil {
			return err
		}
	}
	return nil
}

func (sc *SmartContract) accountBalance(db *model.DbTransaction, fid, tid int64, eco int64) (fb, tb decimal.Decimal, err error) {
	if fid == tid {
		toKey := &model.Key{}
		_, err = toKey.SetTablePrefix(eco).GetTr(db, tid)
		if err != nil {
			sc.GetLogger().WithFields(log.Fields{"type": consts.ParameterExceeded, "token_ecosystem": eco, "wallet": tid}).Error("get key balance")
			return
		}
		tb, _ = decimal.NewFromString(toKey.Amount)
		fb = tb
		toKey.SetTablePrefix(eco)
		return
	}

	fromKey := &model.Key{}
	_, err = fromKey.SetTablePrefix(eco).GetTr(db, fid)
	if err != nil {
		sc.GetLogger().WithFields(log.Fields{"type": consts.ParameterExceeded, "token_ecosystem": eco, "wallet": tid}).Error("get key balance")
		return
	}
	fb, _ = decimal.NewFromString(fromKey.Amount)
	toKey := &model.Key{}
	_, err = toKey.SetTablePrefix(eco).GetTr(db, tid)
	if err != nil {
		sc.GetLogger().WithFields(log.Fields{"type": consts.ParameterExceeded, "token_ecosystem": eco, "wallet": tid}).Error("get key balance")
		return
	}
	tb, _ = decimal.NewFromString(toKey.Amount)
	return
}

func (sc *SmartContract) payTaxes(toID string, sum decimal.Decimal, t, eco int64, fromIDStr, comment string) error {
	walletTable := model.KeyTableName(consts.DefaultTokenEcosystem)

	if _, _, err := sc.updateWhere(
		[]string{"+amount"}, []interface{}{sum}, walletTable,
		types.LoadMap(map[string]interface{}{
			"id":        toID,
			"ecosystem": eco,
		})); err != nil {
		return err
	}
	if _, _, err := sc.updateWhere(
		[]string{`-amount`}, []interface{}{sum}, walletTable,
		types.LoadMap(map[string]interface{}{
			`id`:        fromIDStr,
			`ecosystem`: eco,
		})); err != nil {
		return errTaxes
	}
	fromIDBalance, toIDBalance, err := sc.accountBalance(sc.DbTransaction, converter.StrToInt64(fromIDStr), converter.StrToInt64(toID), eco)
	if err != nil {
		return err
	}
	_, _, err = sc.insert(
		[]string{
			"sender_id",
			"recipient_id",
			"sender_balance",
			"recipient_balance",
			"amount",
			"comment",
			"block_id",
			"txhash",
			"ecosystem",
			"type",
			"created_at",
		},
		[]interface{}{
			fromIDStr,
			toID,
			fromIDBalance,
			toIDBalance,
			sum,
			comment,
			sc.BlockData.BlockID,
			sc.TxHash,
			eco,
			t,
			sc.BlockData.Time,
		},
		`1_history`)

	if err != nil {
		return err
	}

	return nil
}

func (sc *SmartContract) needPayment() bool {
	return sc.TxSmart.EcosystemID > 0 && !sc.CLB && !syspar.IsPrivateBlockchain() && sc.payFreeContract()
}

func (sc *SmartContract) prepareMultiPay() (err error) {
	var pay struct {
		toID                                  int64
		fromID                                int64
		fuelRate, storageFuel, newElementFuel decimal.Decimal
		payWallet                             model.Key
		tokenEco                              int64
	}
	cntrctOwnerInfo := sc.TxContract.Block.Info.(*script.ContractInfo).Owner
	err = sc.appendTokens(cntrctOwnerInfo.TokenID, sc.TxSmart.EcosystemID)
	if err != nil {
		return
	}
	var isEcosysWallet = make(map[int64]bool)
	for _, eco := range sc.getTokenEcos() {
		zero := decimal.New(0, 0)
		pay.tokenEco = eco
		if !sc.CLB {
			pay.toID = sc.BlockData.KeyID
			pay.fromID = sc.TxSmart.KeyID
		}
		if cntrctOwnerInfo.WalletID != 0 {
			pay.fromID = cntrctOwnerInfo.WalletID
		}

		if _, ok := syspar.IsFuelRate(eco); !ok {
			fuels := make([][]string, 0)
			err = json.Unmarshal([]byte(syspar.SysString(syspar.FuelRate)), &fuels)
			if err != nil {
				return err
			}
			follow, _ := decimal.NewFromString(syspar.GetFuelRate(consts.DefaultTokenEcosystem))
			followFuel := &model.StateParameter{}
			if foundFollowFuel, _ := followFuel.SetTablePrefix(converter.Int64ToStr(sc.TxSmart.EcosystemID)).Get(sc.DbTransaction, "follow_fuel"); foundFollowFuel && len(followFuel.Value) > 0 {
				times, _ := decimal.NewFromString(followFuel.Value)
				if times.LessThanOrEqual(zero) {
					times = decimal.New(1, 0)
				}
				follow = follow.Mul(times)
			}
			var newFuel []string
			newFuel = append(newFuel, strconv.FormatInt(eco, 10), follow.String())
			fuels = append(fuels, newFuel)
			fuel, err := json.Marshal(fuels)
			if err != nil {
				return err
			}
			sc.taxes = true
			_, err = UpdateSysParam(sc, syspar.FuelRate, string(fuel), "")
			if err != nil {
				return err
			}
		}
		pay.fuelRate, _ = decimal.NewFromString(syspar.GetFuelRate(eco))
		if pay.fuelRate.Cmp(zero) <= 0 {
			sc.GetLogger().WithFields(log.Fields{"type": consts.ParameterExceeded, "token_ecosystem": eco}).Error("fuel rate must be greater than 0")
			err = fmt.Errorf(eEcoFuelRate, eco)
			return
		}
		if _, ok := syspar.IsTaxesWallet(eco); !ok {
			var taxesPub []byte
			err = model.GetDB(sc.DbTransaction).Select("pub").Model(&model.Key{}).Where("id = ? AND ecosystem = 1", syspar.GetTaxesWallet(1)).Row().Scan(&taxesPub)
			if err != nil {
				return err
			}
			id := PubToID(fmt.Sprintf("%x", taxesPub))
			key := &model.Key{}
			found, err := key.SetTablePrefix(eco).Get(sc.DbTransaction, id)
			if err != nil {
				return err
			}
			if !found {
				_, _, err = DBInsert(sc, "@1keys", types.LoadMap(map[string]interface{}{
					"id":      id,
					"account": IDToAddress(id),
					"amount":  0, "ecosystem": eco}))
				if err != nil {
					return err
				}
			}

			taxes := make([][]string, 0)
			err = json.Unmarshal([]byte(syspar.SysString(syspar.TaxesWallet)), &taxes)
			if err != nil {
				return err
			}
			var newTaxes []string
			newTaxes = append(newTaxes, strconv.FormatInt(eco, 10), strconv.FormatInt(id, 10))
			taxes = append(taxes, newTaxes)
			tax, err := json.Marshal(taxes)
			if err != nil {
				return err
			}
			sc.taxes = true
			_, err = UpdateSysParam(sc, syspar.TaxesWallet, string(tax), "")
			if err != nil {
				return err
			}
		}
		key := &model.Key{}
		var found bool
		if found, err = key.SetTablePrefix(eco).Get(sc.DbTransaction, pay.toID); err != nil || !found {
			if err != nil {
				return err
			}
			if !found {
				_, _, err = DBInsert(sc, "@1keys", types.LoadMap(map[string]interface{}{
					"id":      pay.toID,
					"account": IDToAddress(pay.toID),
					"amount":  0, "ecosystem": eco}))
				if err != nil {
					return err
				}
			}
		}
		if sc.TxSmart.EcosystemID != consts.DefaultTokenEcosystem {
			if eco != consts.DefaultTokenEcosystem {
				ew := &model.StateParameter{}
				if foundEcosystemWallet, _ := ew.SetTablePrefix(converter.Int64ToStr(sc.TxSmart.EcosystemID)).Get(sc.DbTransaction, "ecosystem_wallet"); foundEcosystemWallet && len(ew.Value) > 0 {
					ecosystemWallet := AddressToID(ew.Value)
					if ecosystemWallet != 0 {
						pay.fromID = ecosystemWallet
						isEcosysWallet[eco] = true
					}
				}
			}
			if len(sc.getTokenEcos()) == 1 || eco != consts.DefaultTokenEcosystem {
				pay.storageFuel = decimal.New(syspar.SysInt64(syspar.PriceTxSizeWallet), 0).
					Mul(decimal.New(syspar.SysInt64(syspar.PriceCreateRate), 0)).
					Mul(pay.fuelRate).Mul(decimal.New(sc.TxSize, 0)).
					Div(decimal.NewFromInt(consts.ChainSize)).Floor()
				if pay.storageFuel.LessThanOrEqual(zero) {
					pay.storageFuel = decimal.New(1, 0)
				}
			}
		}
		if found, err = pay.payWallet.SetTablePrefix(eco).Get(sc.DbTransaction, pay.fromID); err != nil || !found {
			if !found {
				err = fmt.Errorf(eEcoKeyNotFound, converter.AddressToString(pay.fromID), eco)
				sc.GetLogger().WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("looking for keyid in ecosystem")
				return
			}
			sc.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting wallet")
			return
		}
		amount := pay.payWallet.CapableAmount()
		if cntrctOwnerInfo.WalletID == 0 && !isEcosysWallet[eco] &&
			!bytes.Equal(sc.Key.PublicKey, pay.payWallet.PublicKey) &&
			!bytes.Equal(sc.TxSmart.PublicKey, pay.payWallet.PublicKey) &&
			sc.TxSmart.SignedBy == 0 {
			err = errDiffKeys
			sc.GetLogger().WithFields(log.Fields{"type": consts.ParameterExceeded, "error": err}).Error(errDiffKeys)
			return
		}
		if eco == consts.DefaultTokenEcosystem {
			if priceName, ok := script.ContractPrices[sc.TxContract.Name]; ok {
				newElementPrices := decimal.NewFromInt(SysParamInt(priceName)).
					Mul(decimal.NewFromInt(syspar.SysInt64(syspar.PriceCreateRate))).
					Mul(pay.fuelRate)
				if newElementPrices.GreaterThan(decimal.New(MaxPrice, 0)) {
					sc.GetLogger().WithFields(log.Fields{"type": consts.NoFunds}).Error("Price value is more than the highest value")
					err = errMaxPrice
					return
				}
				if newElementPrices.LessThan(zero) {
					sc.GetLogger().WithFields(log.Fields{"type": consts.NoFunds}).Error("Price value is negative")
					err = errNegPrice
					return
				}
				pay.newElementFuel = newElementPrices
			}
		}
		var estimate decimal.Decimal
		if len(sc.TxSmart.Expedite) > 0 {
			expedite, _ := decimal.NewFromString(sc.TxSmart.Expedite)
			if expedite.LessThan(zero) {
				err = fmt.Errorf(eGreaterThan, sc.TxSmart.Expedite)
				return
			}
			estimate = estimate.Add(StringToAmount(sc.TxSmart.Expedite))
		}
		estimate = estimate.Add(pay.newElementFuel).Add(pay.storageFuel)
		if amount.LessThan(estimate) {
			difference, _ := FormatMoney(sc, estimate.Sub(amount).String(), consts.MoneyDigits)
			err = fmt.Errorf(eEcoCurrentBalance, eco, difference)
			sc.GetLogger().WithFields(log.Fields{"type": consts.NoFunds, "token_eco": eco, "difference": difference}).Error("current balance is not enough")
			return
		}
		sc.multiPays = append(sc.multiPays, pay)
	}
	return
}

func (sc *SmartContract) appendTokens(nums ...int64) error {
	sc.TxSmart.TokenEcosystems = make(map[int64]interface{})
	if len(sc.TxSmart.TokenEcosystems) == 0 {
		sc.TxSmart.TokenEcosystems[consts.DefaultTokenEcosystem] = nil
	}

	for _, num := range nums {
		if num <= 1 {
			continue
		}
		if _, ok := sc.TxSmart.TokenEcosystems[num]; ok {
			continue
		}
		ecosystems := model.Ecosystem{}
		_, err := ecosystems.Get(sc.DbTransaction, num)
		if err != nil {
			return err
		}
		if !ecosystems.IsOpenMultiFee() {
			return nil
		}
		if len(ecosystems.TokenTitle) <= 0 {
			return nil
		}
		if _, ok := sc.TxSmart.TokenEcosystems[num]; !ok {
			sc.TxSmart.TokenEcosystems[num] = nil
		}
	}
	return nil
}

func (sc *SmartContract) getTokenEcos() []int64 {
	var ecos []int64
	for i := range sc.TxSmart.TokenEcosystems {
		ecos = append(ecos, i)
	}
	return ecos
}

func (sc *SmartContract) payFreeContract() bool {
	var (
		pfca  []string
		ispay bool
	)

	pfc := syspar.SysString(syspar.PayFreeContract)
	if len(pfc) > 0 {
		pfca = strings.Split(pfc, ",")
	}
	for _, value := range pfca {
		if strings.TrimSpace(value) == sc.TxContract.Name {
			ispay = true
			break
		}
	}
	return !ispay
}
