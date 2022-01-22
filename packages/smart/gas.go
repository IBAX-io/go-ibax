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
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

const (
	paymentContractCaller = iota + 1
	paymentContractBinder
	paymentEcosystem

	vmCostFeeCategory = iota + 1
	storageFeeCategory
	elementFeeCategory
	expediteFeeCategory
)

type (
	fuelCategory struct {
		fuelType   int
		decimal    decimal.Decimal
		percentage int
	}

	fromIDInfo struct {
		paymentType int
		fromID      int64
		fuelRate    decimal.Decimal
		vmCostFee   *fuelCategory
		storageFee  *fuelCategory
		elementFee  *fuelCategory
		expediteFee *fuelCategory
		payWallet   *sqldb.Key
	}

	paymentInfo struct {
		tokenEco    int64
		toID        int64
		taxes       string
		fromIDInfos []*fromIDInfo
	}
	multiPays []*paymentInfo
)

func newFuelCategory(fuelType int, decimal decimal.Decimal, percentage int) *fuelCategory {
	return &fuelCategory{fuelType: fuelType, decimal: decimal, percentage: percentage}
}

func (f *fuelCategory) Fees() decimal.Decimal {
	return f.decimal.Mul(decimal.NewFromInt(int64(f.percentage))).Div(decimal.NewFromInt(100)).Floor()
}

func (f *fromIDInfo) checkVerify(sc *SmartContract, eco int64) error {
	if found, err := f.payWallet.SetTablePrefix(eco).Get(sc.DbTransaction, f.fromID); err != nil || !found {
		if !found {
			sc.GetLogger().WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("looking for keyid in ecosystem")
			return fmt.Errorf(eEcoKeyNotFound, converter.AddressToString(f.fromID), eco)
		}
		sc.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting wallet")
		return err
	}
	if f.paymentType == paymentContractCaller &&
		!bytes.Equal(sc.Key.PublicKey, f.payWallet.PublicKey) &&
		!bytes.Equal(sc.TxSmart.PublicKey, f.payWallet.PublicKey) &&
		sc.TxSmart.SignedBy == 0 {
		sc.GetLogger().WithFields(log.Fields{"type": consts.ParameterExceeded, "error": errDiffKeys}).Error(errDiffKeys)
		return errDiffKeys
	}
	var estimate decimal.Decimal
	estimate = estimate.Add(f.elementFee.Fees()).
		Add(f.storageFee.Fees()).
		Add(f.expediteFee.Fees())
	amount := f.payWallet.CapableAmount()
	if amount.LessThan(estimate) {
		difference, _ := FormatMoney(sc, estimate.Sub(amount).String(), consts.MoneyDigits)
		sc.GetLogger().WithFields(log.Fields{"type": consts.NoFunds, "token_eco": eco, "difference": difference}).Error("current balance is not enough")
		return fmt.Errorf(eEcoCurrentBalance, eco, difference)
	}
	return nil
}

func (sc *SmartContract) payContract(errNeedPay bool) error {
	sc.Penalty = errNeedPay
	placeholder := `taxes for execution of %s contract`
	comment := fmt.Sprintf(placeholder, sc.TxContract.Name)
	for _, pay := range sc.multiPays {
		for i := 0; i < len(pay.fromIDInfos); i++ {
			frominfo := pay.fromIDInfos[i]
			frominfo.vmCostFee = newFuelCategory(vmCostFeeCategory, sc.TxUsedCost.Mul(frominfo.fuelRate), frominfo.vmCostFee.percentage)
			money := frominfo.vmCostFee.Fees().Add(frominfo.storageFee.Fees()).Add(frominfo.expediteFee.Fees())
			if !errNeedPay {
				money = money.Add(frominfo.elementFee.Fees())
			} else {
				comment = "(error)" + comment
				ts := sqldb.TransactionStatus{}
				if err := ts.UpdatePenalty(sc.DbTransaction, sc.TxHash); err != nil {
					return err
				}
			}
			wltAmount := frominfo.payWallet.CapableAmount()
			if wltAmount.Cmp(money) < 0 {
				return errTaxes
			}
			taxes := money.Mul(decimal.New(syspar.SysInt64(syspar.TaxesSize), 0)).Div(decimal.New(100, 0)).Floor()
			fromIDStr := converter.Int64ToStr(frominfo.fromID)
			if err := sc.payTaxes(converter.Int64ToStr(pay.toID), money.Sub(taxes), 1, pay.tokenEco, fromIDStr, comment); err != nil {
				return err
			}

			if err := sc.payTaxes(pay.taxes, taxes, 2, pay.tokenEco, fromIDStr, comment); err != nil {
				return err
			}
		}
	}
	return nil
}

func (sc *SmartContract) accountBalance(db *sqldb.DbTransaction, fid, tid int64, eco int64) (fb, tb decimal.Decimal, err error) {
	if fid == tid {
		toKey := &sqldb.Key{}
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

	fromKey := &sqldb.Key{}
	_, err = fromKey.SetTablePrefix(eco).GetTr(db, fid)
	if err != nil {
		sc.GetLogger().WithFields(log.Fields{"type": consts.ParameterExceeded, "token_ecosystem": eco, "wallet": tid}).Error("get key balance")
		return
	}
	fb, _ = decimal.NewFromString(fromKey.Amount)
	toKey := &sqldb.Key{}
	_, err = toKey.SetTablePrefix(eco).GetTr(db, tid)
	if err != nil {
		sc.GetLogger().WithFields(log.Fields{"type": consts.ParameterExceeded, "token_ecosystem": eco, "wallet": tid}).Error("get key balance")
		return
	}
	tb, _ = decimal.NewFromString(toKey.Amount)
	return
}

func (sc *SmartContract) payTaxes(toID string, sum decimal.Decimal, t, eco int64, fromIDStr, comment string) error {
	walletTable := sqldb.KeyTableName(consts.DefaultTokenEcosystem)

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

func (sc *SmartContract) hasExitKeyID(eco, id int64) error {
	var (
		found bool
		err   error
	)
	key := &sqldb.Key{}
	found, err = key.SetTablePrefix(eco).Get(sc.DbTransaction, id)
	if err != nil {
		return err
	}
	if !found {
		_, _, err = DBInsert(sc, "@1keys", types.LoadMap(map[string]interface{}{
			"id":        id,
			"account":   IDToAddress(id),
			"amount":    0,
			"ecosystem": eco,
		}))
		if err != nil {
			return err
		}
	}
	return nil
}

func (sc *SmartContract) getFromIdAndPayType(eco int64) (int64, int) {
	var (
		ownerInfo   = sc.TxContract.Info().Owner
		paymentType int
		fromID      int64
	)

	paymentType = paymentContractCaller
	fromID = sc.TxSmart.KeyID

	if ownerInfo.WalletID != 0 {
		paymentType = paymentContractBinder
		fromID = ownerInfo.WalletID
		return fromID, paymentType
	}

	if sc.TxSmart.EcosystemID != consts.DefaultTokenEcosystem && eco != consts.DefaultTokenEcosystem {
		ew := &sqldb.StateParameter{}
		if found, _ := ew.SetTablePrefix(converter.Int64ToStr(sc.TxSmart.EcosystemID)).
			Get(sc.DbTransaction, sqldb.EcosystemWallet); found && len(ew.Value) > 0 {
			ecosystemWallet := AddressToID(ew.Value)
			if ecosystemWallet != 0 {
				paymentType = paymentEcosystem
				fromID = ecosystemWallet
			}
		}
	}

	return fromID, paymentType
}

func (sc *SmartContract) getChangeAddress(eco int64) (fromIDInfos []*fromIDInfo, err error) {
	var (
		fuelRate    decimal.Decimal
		storageFee  decimal.Decimal
		elementFee  decimal.Decimal
		expediteFee decimal.Decimal
	)

	ecosystems := &sqldb.Ecosystem{}
	_, err = ecosystems.Get(sc.DbTransaction, eco)
	if err != nil {
		return
	}
	feeMode := ecosystems.FeeMode()

	if fuelRate, err = sc.fuelRate(eco); err != nil {
		return
	}
	if elementFee, err = sc.elementFee(eco, fuelRate); err != nil {
		return
	}
	if expediteFee, err = sc.expediteFee(); err != nil {
		return
	}

	storageFee = sc.storageFee(eco, fuelRate)

	var from = &fromIDInfo{
		payWallet:   &sqldb.Key{},
		fuelRate:    fuelRate,
		vmCostFee:   new(fuelCategory),
		storageFee:  new(fuelCategory),
		elementFee:  new(fuelCategory),
		expediteFee: new(fuelCategory),
	}
	from.fromID, from.paymentType = sc.getFromIdAndPayType(eco)

	for k, v := range feeMode {
		switch k {
		case sqldb.FeeModeVmCost:
			from.vmCostFee = newFuelCategory(vmCostFeeCategory, decimal.NewFromInt(0), v)
		case sqldb.FeeModeStorage:
			from.storageFee = newFuelCategory(storageFeeCategory, storageFee, v)
		case sqldb.FeeModeElement:
			from.elementFee = newFuelCategory(elementFeeCategory, elementFee, v)
		case sqldb.FeeModeExpedite:
			from.expediteFee = newFuelCategory(expediteFeeCategory, expediteFee, v)
		}
	}

	if from.vmCostFee.fuelType == 0 {
		from.vmCostFee = newFuelCategory(vmCostFeeCategory, decimal.NewFromInt(0), 100)
	}
	if from.storageFee.fuelType == 0 {
		from.storageFee = newFuelCategory(storageFeeCategory, storageFee, 100)
	}
	if from.elementFee.fuelType == 0 {
		from.elementFee = newFuelCategory(elementFeeCategory, elementFee, 100)
	}
	if from.expediteFee.fuelType == 0 {
		from.expediteFee = newFuelCategory(expediteFeeCategory, expediteFee, 100)
	}

	if err = from.checkVerify(sc, eco); err != nil {
		return
	}

	fromIDInfos = append(fromIDInfos, from)
	return
}

func (sc *SmartContract) prepareMultiPay() (err error) {
	ownerInfo := sc.TxContract.Info().Owner
	if err = sc.appendTokens(ownerInfo.TokenID, sc.TxSmart.EcosystemID); err != nil {
		return err
	}

	for _, eco := range sc.getTokenEcos() {
		pay := &paymentInfo{
			tokenEco: eco,
			toID:     sc.BlockData.KeyID,
		}
		if err = sc.hasExitKeyID(eco, pay.toID); err != nil {
			return err
		}
		if err = sc.taxesWallet(eco); err != nil {
			return err
		}
		pay.taxes = syspar.GetTaxesWallet(pay.tokenEco)
		pay.fromIDInfos, err = sc.getChangeAddress(eco)
		if err != nil {
			return err
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
		ecosystems := &sqldb.Ecosystem{}
		_, err := ecosystems.Get(sc.DbTransaction, num)
		if err != nil {
			return err
		}
		if !ecosystems.IsOpenMultiFee() {
			continue
		}
		if len(ecosystems.TokenSymbol) <= 0 {
			continue
		}
		sc.TxSmart.TokenEcosystems[num] = nil
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

func (sc *SmartContract) fuelRate(eco int64) (decimal.Decimal, error) {
	var (
		fuelRate decimal.Decimal
		err      error
		zero     = decimal.New(0, 0)
	)
	if _, ok := syspar.HasFuelRate(eco); !ok {
		fuels := make([][]string, 0)
		err = json.Unmarshal([]byte(syspar.SysString(syspar.FuelRate)), &fuels)
		if err != nil {
			return zero, err
		}
		follow, _ := decimal.NewFromString(syspar.GetFuelRate(consts.DefaultTokenEcosystem))
		followFuel := &sqldb.StateParameter{}
		if found, _ := followFuel.SetTablePrefix(converter.Int64ToStr(sc.TxSmart.EcosystemID)).
			Get(sc.DbTransaction, sqldb.FollowFuel); found && len(followFuel.Value) > 0 {
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
			return zero, err
		}
		sc.taxes = true
		_, err = UpdateSysParam(sc, syspar.FuelRate, string(fuel), "")
		if err != nil {
			return zero, err
		}
	}
	fuelRate, err = decimal.NewFromString(syspar.GetFuelRate(eco))
	if err != nil {
		return zero, err
	}
	if fuelRate.Cmp(zero) <= 0 {
		sc.GetLogger().WithFields(log.Fields{"type": consts.ParameterExceeded, "token_ecosystem": eco}).Error("fuel rate must be greater than 0")
		err = fmt.Errorf(eEcoFuelRate, eco)
		return zero, err
	}
	return fuelRate, nil
}

func (sc *SmartContract) elementFee(eco int64, fuelRate decimal.Decimal) (decimal.Decimal, error) {
	var (
		elementFee decimal.Decimal
		err        error
		zero       = decimal.New(0, 0)
	)
	if priceName, ok := script.ContractPrices[sc.TxContract.Name]; ok && eco == consts.DefaultTokenEcosystem {
		newElementPrices := decimal.NewFromInt(SysParamInt(priceName)).
			Mul(decimal.NewFromInt(syspar.SysInt64(syspar.PriceCreateRate))).
			Mul(fuelRate)
		if newElementPrices.GreaterThan(decimal.New(MaxPrice, 0)) {
			sc.GetLogger().WithFields(log.Fields{"type": consts.NoFunds}).Error("Price value is more than the highest value")
			err = errMaxPrice
			return zero, err
		}
		if newElementPrices.LessThan(zero) {
			sc.GetLogger().WithFields(log.Fields{"type": consts.NoFunds}).Error("Price value is negative")
			err = errNegPrice
			return zero, err
		}
		elementFee = newElementPrices
	}
	return elementFee, err
}

func (sc *SmartContract) storageFee(eco int64, fuelRate decimal.Decimal) decimal.Decimal {
	var (
		storageFee decimal.Decimal
		zero       = decimal.New(0, 0)
	)
	if sc.TxSmart.EcosystemID != consts.DefaultTokenEcosystem {
		if len(sc.getTokenEcos()) == 1 || eco != consts.DefaultTokenEcosystem {
			storageFee = decimal.NewFromInt(syspar.SysInt64(syspar.PriceTxSize)).
				Mul(decimal.NewFromInt(syspar.SysInt64(syspar.PriceCreateRate))).
				Mul(fuelRate).Mul(decimal.NewFromInt(sc.TxSize)).
				Div(decimal.NewFromInt(consts.ChainSize)).Floor()
			if storageFee.LessThanOrEqual(zero) {
				storageFee = decimal.New(1, 0)
			}
			return storageFee
		}
	}
	return zero
}

func (sc *SmartContract) taxesWallet(eco int64) (err error) {
	if _, ok := syspar.HasTaxesWallet(eco); !ok {
		var taxesPub []byte
		err = sqldb.GetDB(sc.DbTransaction).Select("pub").
			Model(&sqldb.Key{}).Where("id = ? AND ecosystem = 1",
			syspar.GetTaxesWallet(1)).Row().Scan(&taxesPub)
		if err != nil {
			return err
		}
		id := PubToID(fmt.Sprintf("%x", taxesPub))
		if err := sc.hasExitKeyID(eco, id); err != nil {
			return err
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
	return
}

func (sc *SmartContract) expediteFee() (decimal.Decimal, error) {
	zero := decimal.New(0, 0)
	if len(sc.TxSmart.Expedite) > 0 {
		expedite, _ := decimal.NewFromString(sc.TxSmart.Expedite)
		if expedite.LessThan(zero) {
			return zero, fmt.Errorf(eGreaterThan, sc.TxSmart.Expedite)
		}
		return StringToAmount(sc.TxSmart.Expedite), nil
	}
	return zero, nil
}
