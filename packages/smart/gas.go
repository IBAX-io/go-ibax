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
	paymentInfo struct {
		tokenEco    int64
		toID        int64
		taxesID     int64
		fromID      int64
		paymentType PaymentType
		fuelRate    decimal.Decimal
		vmCostFee   *fuelCategory
		storageFee  *fuelCategory
		elementFee  *fuelCategory
		expediteFee *fuelCategory
		tokenSymbol string
		payWallet   *sqldb.Key
		taxesSize   int64
		indirect    bool
	}
	multiPays []*paymentInfo
)

func newFuelCategory(fuelType int, decimal decimal.Decimal, percentage int) *fuelCategory {
	return &fuelCategory{fuelType: fuelType, decimal: decimal, percentage: percentage}
}

func (f *fuelCategory) Detail() (string, interface{}) {
	return f.CategoryString(), f.FeesInfo()
}

func (f *fuelCategory) FeesInfo() interface{} {
	detail := types.NewMap()
	detail.Set("decimal", f.decimal)
	detail.Set("value", f.Fees())
	detail.Set("percentage", f.percentage)
	b, _ := JSONEncode(detail)
	s, _ := JSONDecode(b)
	return s
}

func (f *fuelCategory) Fees() decimal.Decimal {
	return f.decimal.Mul(decimal.NewFromInt(int64(f.percentage))).Div(decimal.NewFromInt(100)).Floor()
}

func (f *fuelCategory) CategoryString() string {
	switch f.fuelType {
	case vmCostFeeCategory:
		return "vmCost_fee"
	case storageFeeCategory:
		return "storage_fee"
	case elementFeeCategory:
		return "element_fee"
	case expediteFeeCategory:
		return "expedite_fee"
	default:
		return "others_fee"
	}
}

func (pay *paymentInfo) Detail() interface{} {
	detail := types.NewMap()
	detail.Set(pay.vmCostFee.Detail())
	detail.Set(pay.storageFee.Detail())
	detail.Set(pay.elementFee.Detail())
	detail.Set(pay.expediteFee.Detail())
	detail.Set("taxes_size", pay.taxesSize)
	detail.Set("payment_type", pay.paymentType.String())
	detail.Set("fuel_rate", pay.fuelRate)
	b, _ := JSONEncode(detail)
	s, _ := JSONDecode(b)
	return s
}

func (f *paymentInfo) checkVerify(sc *SmartContract) error {
	eco := f.tokenEco
	if err := sc.hasExitKeyID(eco, f.toID); err != nil {
		return err
	}
	if found, err := f.payWallet.SetTablePrefix(eco).Get(sc.DbTransaction, f.fromID); err != nil || !found {
		if !found {
			sc.GetLogger().WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("looking for keyid in ecosystem")
			return fmt.Errorf(eEcoKeyNotFound, converter.AddressToString(f.fromID), eco)
		}
		sc.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting wallet")
		return err
	}
	if f.paymentType == PaymentType_ContractCaller &&
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
	if errNeedPay {
		comment = "(error)" + comment
		ts := sqldb.TransactionStatus{}
		if err := ts.UpdatePenalty(sc.DbTransaction, sc.Hash); err != nil {
			return err
		}
	}
	for i := 0; i < len(sc.multiPays); i++ {
		pay := sc.multiPays[i]
		pay.vmCostFee = newFuelCategory(vmCostFeeCategory, sc.TxUsedCost.Mul(pay.fuelRate), pay.vmCostFee.percentage)
		money := pay.vmCostFee.Fees().Add(pay.storageFee.Fees()).Add(pay.expediteFee.Fees())
		if !errNeedPay {
			money = money.Add(pay.elementFee.Fees())
		}
		wltAmount := pay.payWallet.CapableAmount()
		if wltAmount.Cmp(money) < 0 {
			return errTaxes
		}
		if pay.indirect {
			if err := sc.payTaxes(pay, money, 15, comment); err != nil {
				return err
			}
		} else {
			taxes := money.Mul(decimal.New(pay.taxesSize, 0)).Div(decimal.New(100, 0)).Floor()
			if err := sc.payTaxes(pay, money.Sub(taxes), 1, comment); err != nil {
				return err
			}
			if err := sc.payTaxes(pay, taxes, 2, comment); err != nil {
				return err
			}
		}
	}
	return nil
}

func (sc *SmartContract) accountBalanceSingle(db *sqldb.DbTransaction, id, eco int64) (decimal.Decimal, error) {
	key := &sqldb.Key{}
	_, err := key.SetTablePrefix(eco).GetTr(db, id)
	if err != nil {
		sc.GetLogger().WithFields(log.Fields{"type": consts.ParameterExceeded, "token_ecosystem": eco, "wallet": id}).Error("get key balance")
		return decimal.Zero, err
	}
	balance, _ := decimal.NewFromString(key.Amount)
	return balance, nil
}

func (sc *SmartContract) payTaxes(pay *paymentInfo, sum decimal.Decimal, t int64, comment string) error {
	var toID int64
	if t == 1 || t == 15 {
		toID = pay.toID
	}
	if t == 2 {
		toID = pay.taxesID
	}
	if sum.IsZero() {
		return nil
	}
	if _, _, err := sc.updateWhere(
		[]string{`-amount`}, []interface{}{sum}, "1_keys",
		types.LoadMap(map[string]interface{}{
			`id`:        pay.fromID,
			`ecosystem`: pay.tokenEco,
		})); err != nil {
		return errTaxes
	}
	if _, _, err := sc.updateWhere(
		[]string{"+amount"}, []interface{}{sum}, "1_keys",
		types.LoadMap(map[string]interface{}{
			"id":        toID,
			"ecosystem": pay.tokenEco,
		})); err != nil {
		return err
	}
	var (
		values *types.Map
		fromIDBalance,
		toIDBalance decimal.Decimal
		err error
	)

	if fromIDBalance, err = sc.accountBalanceSingle(sc.DbTransaction, pay.fromID, pay.tokenEco); err != nil {
		return err
	}

	if toIDBalance, err = sc.accountBalanceSingle(sc.DbTransaction, toID, pay.tokenEco); err != nil {
		return err
	}
	values = types.LoadMap(map[string]interface{}{
		"sender_id":         pay.fromID,
		"sender_balance":    fromIDBalance,
		"recipient_id":      toID,
		"recipient_balance": toIDBalance,
		"amount":            sum,
		"comment":           comment,
		"block_id":          sc.BlockData.BlockID,
		"txhash":            sc.Hash,
		"ecosystem":         pay.tokenEco,
		"type":              t,
		"created_at":        sc.Timestamp,
	})
	if t == 1 || t == 15 {
		values.Set("value_detail", pay.Detail())
	}

	_, _, err = sc.insert(values.Keys(), values.Values(), `1_history`)
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

func (sc *SmartContract) getFromIdAndPayType(eco int64) (int64, PaymentType) {
	var (
		ownerInfo   = sc.TxContract.Info().Owner
		paymentType PaymentType
		fromID      int64
	)

	paymentType = PaymentType_ContractCaller
	fromID = sc.TxSmart.KeyID

	if ownerInfo.WalletID != 0 {
		paymentType = PaymentType_ContractBinder
		fromID = ownerInfo.WalletID
		return fromID, paymentType
	}

	if sc.TxSmart.EcosystemID != consts.DefaultTokenEcosystem && eco != consts.DefaultTokenEcosystem {
		ew := &sqldb.StateParameter{}
		if found, _ := ew.SetTablePrefix(converter.Int64ToStr(sc.TxSmart.EcosystemID)).
			Get(sc.DbTransaction, sqldb.EcosystemWallet); found && len(ew.Value) > 0 {
			ecosystemWallet := AddressToID(ew.Value)
			if ecosystemWallet != 0 {
				paymentType = PaymentType_EcosystemAddress
				fromID = ecosystemWallet
			}
		}
	}

	return fromID, paymentType
}

func (sc *SmartContract) getChangeAddress(eco int64) ([]*paymentInfo, error) {
	var (
		err         error
		storageFee  decimal.Decimal
		elementFee  decimal.Decimal
		expediteFee decimal.Decimal
		pays        []*paymentInfo
	)

	var pay = &paymentInfo{
		tokenEco:    eco,
		toID:        sc.BlockData.KeyID,
		payWallet:   &sqldb.Key{},
		vmCostFee:   new(fuelCategory),
		storageFee:  new(fuelCategory),
		elementFee:  new(fuelCategory),
		expediteFee: new(fuelCategory),
		taxesSize:   syspar.SysInt64(syspar.TaxesSize),
	}

	if pay.fuelRate, err = sc.fuelRate(eco); err != nil {
		return nil, err
	}
	if err := sc.taxesWallet(eco); err != nil {
		return nil, err
	}
	pay.taxesID = converter.StrToInt64(syspar.GetTaxesWallet(pay.tokenEco))
	if elementFee, err = sc.elementFee(eco, pay.fuelRate); err != nil {
		return nil, err
	}

	if expediteFee, err = sc.expediteFee(); err != nil {
		return nil, err
	}

	storageFee = sc.storageFee(pay.fuelRate)
	pay.fromID, pay.paymentType = sc.getFromIdAndPayType(eco)

	ecosystems := &sqldb.Ecosystem{}
	if _, err = ecosystems.Get(sc.DbTransaction, eco); err != nil {
		return nil, err
	}
	if pay.tokenEco == consts.DefaultTokenEcosystem {
		pay.tokenSymbol = "IBXC"
	} else {
		pay.tokenSymbol = ecosystems.TokenSymbol
	}
	feeMode := ecosystems.FeeMode()
	if _, ok := feeMode[sqldb.FeeModeType]; ok && pay.paymentType != PaymentType_ContractCaller {
		indirectPay := &paymentInfo{
			indirect:    true,
			tokenEco:    pay.tokenEco,
			tokenSymbol: pay.tokenSymbol,
			toID:        pay.fromID,
			fromID:      sc.TxSmart.KeyID,
			paymentType: pay.paymentType,
			fuelRate:    pay.fuelRate,
			payWallet:   &sqldb.Key{},
		}
		if pay.fuelRate, err = sc.fuelRate(1); err != nil {
			return nil, err
		}
		pay.tokenEco = consts.DefaultTokenEcosystem
		pay.tokenSymbol = "IBXC"
		if v, ok := feeMode[sqldb.FeeModeVmCost]; ok {
			indirectPay.vmCostFee = newFuelCategory(vmCostFeeCategory, decimal.NewFromInt(0), v)
		} else {
			indirectPay.expediteFee = newFuelCategory(vmCostFeeCategory, decimal.NewFromInt(0), 100)
		}
		if v, ok := feeMode[sqldb.FeeModeStorage]; ok {
			indirectPay.storageFee = newFuelCategory(storageFeeCategory, storageFee, v)
		} else {
			indirectPay.expediteFee = newFuelCategory(storageFeeCategory, storageFee, 100)
		}
		if v, ok := feeMode[sqldb.FeeModeElement]; ok {
			indirectPay.elementFee = newFuelCategory(elementFeeCategory, elementFee, v)
		} else {
			indirectPay.expediteFee = newFuelCategory(elementFeeCategory, elementFee, 100)
		}
		if v, ok := feeMode[sqldb.FeeModeExpedite]; ok {
			indirectPay.expediteFee = newFuelCategory(expediteFeeCategory, expediteFee, v)
		} else {
			indirectPay.expediteFee = newFuelCategory(expediteFeeCategory, expediteFee, 100)
		}
		if err = indirectPay.checkVerify(sc); err != nil {
			return nil, err
		}
		pays = append(pays, indirectPay)
	}

	if pay.vmCostFee.fuelType == 0 {
		pay.vmCostFee = newFuelCategory(vmCostFeeCategory, decimal.NewFromInt(0), 100)
	}
	if pay.storageFee.fuelType == 0 {
		pay.storageFee = newFuelCategory(storageFeeCategory, storageFee, 100)
	}
	if pay.elementFee.fuelType == 0 {
		pay.elementFee = newFuelCategory(elementFeeCategory, elementFee, 100)
	}
	if pay.expediteFee.fuelType == 0 {
		pay.expediteFee = newFuelCategory(expediteFeeCategory, expediteFee, 100)
	}

	if err = pay.checkVerify(sc); err != nil {
		return nil, err
	}
	pays = append(pays, pay)

	return pays, err
}

func (sc *SmartContract) prepareMultiPay() error {
	ownerInfo := sc.TxContract.Info().Owner
	if err := sc.appendTokens(ownerInfo.TokenID, sc.TxSmart.EcosystemID); err != nil {
		return err
	}

	for _, eco := range sc.getTokenEcos() {
		pays, err := sc.getChangeAddress(eco)
		if err != nil {
			return err
		}
		sc.multiPays = append(sc.multiPays, pays...)
	}
	return nil
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
		zero     = decimal.Zero
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
		_, err = UpdatePlatformParam(sc, syspar.FuelRate, string(fuel), "")
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
		zero       = decimal.Zero
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

func (sc *SmartContract) storageFee(fuelRate decimal.Decimal) decimal.Decimal {
	var (
		storageFee decimal.Decimal
		zero       = decimal.Zero
	)
	storageFee = decimal.NewFromInt(syspar.SysInt64(syspar.PriceTxSize)).
		Mul(decimal.NewFromInt(syspar.SysInt64(syspar.PriceCreateRate))).
		Mul(fuelRate).Mul(decimal.NewFromInt(sc.TxSize)).
		Div(decimal.NewFromInt(consts.ChainSize)).Floor()
	if storageFee.LessThanOrEqual(zero) {
		storageFee = decimal.New(1, 0)
	}
	return storageFee
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
		_, err = UpdatePlatformParam(sc, syspar.TaxesWallet, string(tax), "")
		if err != nil {
			return err
		}
	}
	return
}

func (sc *SmartContract) expediteFee() (decimal.Decimal, error) {
	zero := decimal.Zero
	if len(sc.TxSmart.Expedite) > 0 {
		expedite, _ := decimal.NewFromString(sc.TxSmart.Expedite)
		if expedite.LessThan(zero) {
			return zero, fmt.Errorf(eGreaterThan, sc.TxSmart.Expedite)
		}
		return StringToAmount(sc.TxSmart.Expedite), nil
	}
	return zero, nil
}
