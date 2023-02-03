package smart

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/gogo/protobuf/sortkeys"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

type (
	FuelCategory struct {
		FuelType       FuelType
		Decimal        decimal.Decimal
		ConversionRate float64
		Flag           GasPayAbleType
		Arithmetic     Arithmetic
	}
	Combustion struct {
		Flag    int64
		Percent int64
	}
	PaymentInfo struct {
		TokenEco       int64
		ToID           int64
		TaxesID        int64
		FromID         int64
		PaymentType    PaymentType
		FuelRate       decimal.Decimal
		FuelCategories []FuelCategory
		PayWallet      *sqldb.Key
		Ecosystem      *sqldb.Ecosystem
		TaxesSize      int64
		Indirect       bool
		Combustion     *Combustion
		Penalty        bool
	}
	multiPays []*PaymentInfo
)

func newCombustion(flag int64, percent int64) *Combustion {
	if flag <= 0 {
		flag = 1
	}
	if flag == 1 && percent < 0 {
		percent = 0
	}
	return &Combustion{Flag: flag, Percent: percent}
}

func NewFuelCategory(fuelType FuelType, decimal decimal.Decimal, flag GasPayAbleType, convert float64) FuelCategory {
	f := new(FuelCategory)
	f.writeFuelType(fuelType)
	f.writeDecimal(decimal)
	f.writeFlag(flag)
	f.writeConversionRate(convert)
	return *f
}

func (f *FuelCategory) writeFuelType(fuelType FuelType)      { f.FuelType = fuelType }
func (f *FuelCategory) writeDecimal(decimal decimal.Decimal) { f.Decimal = decimal }
func (f *FuelCategory) writeArithmetic(a Arithmetic)         { f.Arithmetic = a }
func (f *FuelCategory) resetArithmetic()                     { f.Arithmetic = Arithmetic_NATIVE }
func (f *FuelCategory) writeFlag(tf GasPayAbleType) {
	if tf == GasPayAbleType_Invalid {
		tf = GasPayAbleType_Capable
	}
	f.Flag = tf
}

func (f *FuelCategory) writeConversionRate(cr float64) {
	if cr > 0 {
		f.ConversionRate = cr
		return
	}
	f.ConversionRate = 100
}

func (f *FuelCategory) Detail() (string, any) {
	return f.FuelType.String(), f.FeesInfo()
}

func (f *FuelCategory) FeesInfo() any {
	detail := types.NewMap()
	detail.Set("decimal", f.Decimal)
	detail.Set("value", f.Fees())
	detail.Set("conversion_rate", f.ConversionRate)
	detail.Set("flag", f.Flag)
	detail.Set("arithmetic", f.Arithmetic.String())
	b, _ := JSONEncode(detail)
	s, _ := JSONDecode(b)
	return s
}

func (f *FuelCategory) Fees() decimal.Decimal {
	var value decimal.Decimal
	switch f.Arithmetic {
	case Arithmetic_NATIVE:
		value = f.Decimal
	case Arithmetic_MUL:
		value = f.Decimal.Mul(decimal.NewFromFloat(f.ConversionRate).Div(decimal.NewFromFloat(100)))
	case Arithmetic_DIV:
		value = f.Decimal.Div(decimal.NewFromFloat(f.ConversionRate).Div(decimal.NewFromFloat(100)))
	default:
		value = f.Decimal
	}
	return value.Floor()
}

func (c Combustion) Fees(sum decimal.Decimal) decimal.Decimal {
	return sum.Mul(decimal.NewFromInt(c.Percent)).Div(decimal.New(100, 0)).Floor()
}

func (c Combustion) Detail(sum decimal.Decimal) any {
	detail := types.NewMap()
	combustion := c.Fees(sum)
	detail.Set("flag", c.Flag)
	detail.Set("percent", c.Percent)
	detail.Set("before", sum)
	detail.Set("value", combustion)
	detail.Set("after", sum.Sub(combustion))
	b, _ := JSONEncode(detail)
	s, _ := JSONDecode(b)
	return s
}

func (pay *PaymentInfo) PushFuelCategories(fes ...FuelCategory) {
	pay.FuelCategories = append(pay.FuelCategories, fes...)
}

func (pay *PaymentInfo) SetDecimalByType(fuelType FuelType, decimal decimal.Decimal) {
	for i, v := range pay.FuelCategories {
		if v.FuelType == fuelType {
			pay.FuelCategories[i].writeDecimal(decimal)
			break
		}
	}
}

func (pay *PaymentInfo) GetPayMoney() decimal.Decimal {
	var money decimal.Decimal
	for i := 0; i < len(pay.FuelCategories); i++ {
		f := pay.FuelCategories[i]
		money = money.Add(f.Fees())
	}
	return money
}

func (pay *PaymentInfo) GetEstimate() decimal.Decimal {
	var money decimal.Decimal
	for i := 0; i < len(pay.FuelCategories); i++ {
		f := pay.FuelCategories[i]
		if f.FuelType == FuelType_vmCost_fee {
			continue
		}
		money.Add(f.Fees())
	}
	return money
}

func (pay *PaymentInfo) Detail() any {
	detail := types.NewMap()
	for i := 0; i < len(pay.FuelCategories); i++ {
		detail.Set(pay.FuelCategories[i].Detail())
	}
	detail.Set("taxes_size", pay.TaxesSize)
	detail.Set("payment_type", pay.PaymentType.String())
	detail.Set("fuel_rate", pay.FuelRate)
	detail.Set("token_symbol", pay.Ecosystem.TokenSymbol)
	b, _ := JSONEncode(detail)
	s, _ := JSONDecode(b)
	return s
}

func (pay *PaymentInfo) DetailCombustion() any {
	c := pay.Combustion
	detail := types.NewMap()
	var money decimal.Decimal
	for i := 0; i < len(pay.FuelCategories); i++ {
		f := pay.FuelCategories[i]
		s := f.Fees().Mul(decimal.NewFromInt(c.Percent)).Div(decimal.New(100, 0)).Floor()
		detail.Set(f.FuelType.String(), s)
		money = money.Add(f.Fees())
	}
	if !pay.Indirect && pay.TokenEco != consts.DefaultTokenEcosystem {
		detail.Set("combustion", pay.Combustion.Detail(money))
	}
	detail.Set("token_symbol", pay.Ecosystem.TokenSymbol)
	detail.Set("fuel_rate", pay.FuelRate)
	b, _ := JSONEncode(detail)
	s, _ := JSONDecode(b)
	return s
}

func (f *PaymentInfo) checkVerify(sc *SmartContract) error {
	eco := f.TokenEco
	if err := sc.hasExistKeyID(eco, f.FromID); err != nil {
		return errors.Wrapf(err, fmt.Sprintf("From ID %d does not exist", f.ToID))
	}
	if err := sc.hasExistKeyID(eco, f.ToID); err != nil {
		return errors.Wrapf(err, fmt.Sprintf("to ID %d does not exist", f.ToID))
	}
	if err := sc.hasExistKeyID(eco, f.TaxesID); err != nil {
		return errors.Wrapf(err, fmt.Sprintf("taxes ID %d does not exist", f.TaxesID))
	}
	if found, err := f.PayWallet.SetTablePrefix(eco).Get(sc.DbTransaction, f.FromID); err != nil || !found {
		if err != nil {
			sc.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting wallet")
			return err
		}
		err = fmt.Errorf(eEcoKeyNotFound, converter.AddressToString(f.FromID), eco)
		sc.GetLogger().WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("looking for keyid in ecosystem")
		return err
	}
	if f.PaymentType == PaymentType_ContractCaller &&
		!bytes.Equal(sc.Key.PublicKey, f.PayWallet.PublicKey) &&
		!bytes.Equal(sc.TxSmart.PublicKey, f.PayWallet.PublicKey) &&
		sc.TxSmart.SignedBy == 0 {
		sc.GetLogger().WithFields(log.Fields{"type": consts.ParameterExceeded, "error": errDiffKeys}).Error(errDiffKeys)
		return errDiffKeys
	}
	estimate := f.GetEstimate()
	amount := f.PayWallet.CapableAmount()
	if amount.LessThan(estimate) {
		sp := &sqldb.Ecosystem{}
		_, err := sp.Get(sc.DbTransaction, eco)
		if err != nil {
			sc.GetLogger().WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting ecosystem")
			return err
		}
		difference, _ := converter.FormatMoney(estimate.Sub(amount).String(), int32(sp.Digits))
		sc.GetLogger().WithFields(log.Fields{"type": consts.NoFunds, "token_eco": eco, "difference": difference}).Error("current balance is not enough")
		return fmt.Errorf(eEcoCurrentBalanceDiff, f.PayWallet.AccountID, eco, difference)
	}
	return nil
}

func (sc *SmartContract) resetFromIDForNativePay(from int64) *PaymentInfo {
	origin := sc.multiPays[0]
	cpy := &PaymentInfo{
		TokenEco:       origin.TokenEco,
		ToID:           origin.ToID,
		TaxesID:        origin.TaxesID,
		FromID:         from,
		PaymentType:    origin.PaymentType,
		FuelRate:       origin.FuelRate,
		FuelCategories: make([]FuelCategory, 0),
		Ecosystem:      origin.Ecosystem,
		PayWallet:      new(sqldb.Key),
		Combustion:     origin.Combustion,
		TaxesSize:      origin.TaxesSize,
	}
	return cpy
}

func (sc *SmartContract) payContract(errNeedPay bool) error {
	sc.Penalty = errNeedPay
	placeholder := `taxes for execution of %s contract`
	comment := fmt.Sprintf(placeholder, sc.TxContract.Name)
	status := pbgo.TxInvokeStatusCode_SUCCESS
	if errNeedPay {
		comment = "(error)" + comment
		status = pbgo.TxInvokeStatusCode_PENALTY
		ts := sqldb.TransactionStatus{}
		if err := ts.UpdatePenalty(sc.DbTransaction, sc.Hash); err != nil {
			return err
		}
	}
	for i := 0; i < len(sc.multiPays); i++ {
		pay := sc.multiPays[i]
		pay.Penalty = sc.Penalty
		pay.SetDecimalByType(FuelType_vmCost_fee, sc.TxUsedCost.Mul(pay.FuelRate))
		money := pay.GetPayMoney()
		wltAmount := pay.PayWallet.CapableAmount()
		if wltAmount.Cmp(money) < 0 {
			if !errNeedPay {
				return errTaxes
			}
			money = wltAmount
		}
		if pay.Indirect {
			if err := sc.payTaxes(pay, money, GasScenesType_Direct, comment, status); err != nil {
				return err
			}
			continue
		}
		if pay.Combustion.Flag == 2 && pay.TokenEco != consts.DefaultTokenEcosystem {
			combustion := pay.Combustion.Fees(money)
			if err := sc.payTaxes(pay, combustion, GasScenesType_Combustion, comment, status); err != nil {
				return err
			}
			money = money.Sub(combustion)
		}
		taxes := money.Mul(decimal.NewFromInt(pay.TaxesSize)).Div(decimal.New(100, 0)).Floor()
		if err := sc.payTaxes(pay, money.Sub(taxes), GasScenesType_Reward, comment, status); err != nil {
			return err
		}
		if err := sc.payTaxes(pay, taxes, GasScenesType_Taxes, comment, status); err != nil {
			return err
		}
	}
	return nil
}

func (sc *SmartContract) accountBalanceSingle(eco, id int64) (decimal.Decimal, error) {
	key := &sqldb.Key{}
	_, err := key.SetTablePrefix(eco).Get(sc.DbTransaction, id)
	if err != nil {
		sc.GetLogger().WithFields(log.Fields{"type": consts.ParameterExceeded, "token_ecosystem": eco, "wallet": id}).Error("get key balance")
		return decimal.Zero, err
	}
	balance, _ := decimal.NewFromString(key.Amount)
	return balance, nil
}

func (sc *SmartContract) payTaxes(pay *PaymentInfo, sum decimal.Decimal, t GasScenesType, comment string, status pbgo.TxInvokeStatusCode) error {
	if sum.IsZero() {
		return nil
	}
	var toID int64
	switch t {
	case GasScenesType_Reward, GasScenesType_Direct:
		toID = pay.ToID
	case GasScenesType_Combustion:
		toID = 0
	case GasScenesType_Taxes:
		toID = pay.TaxesID
	}
	if err := sc.hasExistKeyID(pay.TokenEco, toID); err != nil {
		return err
	}
	var (
		values *types.Map
		fromIDBalance,
		toIDBalance decimal.Decimal
		err error
	)
	if pay.FromID == toID {
		if fromIDBalance, err = sc.accountBalanceSingle(pay.TokenEco, pay.FromID); err != nil {
			return err
		}
		toIDBalance = fromIDBalance
	} else {
		if _, _, err := sc.updateWhere(
			[]string{`-amount`}, []any{sum}, "1_keys",
			types.LoadMap(map[string]any{
				`id`:        pay.FromID,
				`ecosystem`: pay.TokenEco,
			})); err != nil {
			return errTaxes
		}
		if _, _, err := sc.updateWhere(
			[]string{"+amount"}, []any{sum}, "1_keys",
			types.LoadMap(map[string]any{
				"id":        toID,
				"ecosystem": pay.TokenEco,
			})); err != nil {
			return err
		}
		if fromIDBalance, err = sc.accountBalanceSingle(pay.TokenEco, pay.FromID); err != nil {
			return err
		}
		if toIDBalance, err = sc.accountBalanceSingle(pay.TokenEco, toID); err != nil {
			return err
		}
	}

	values = types.LoadMap(map[string]any{
		"sender_id":         pay.FromID,
		"sender_balance":    fromIDBalance,
		"recipient_id":      toID,
		"recipient_balance": toIDBalance,
		"amount":            sum,
		"comment":           comment,
		"status":            int64(status),
		"block_id":          sc.BlockHeader.BlockId,
		"txhash":            sc.Hash,
		"ecosystem":         pay.TokenEco,
		"type":              int64(t),
		"created_at":        sc.Timestamp,
	})
	if t == GasScenesType_Reward || t == GasScenesType_Direct {
		values.Set("value_detail", pay.Detail())
	}
	if t == GasScenesType_Combustion {
		values.Set("value_detail", pay.DetailCombustion())
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

func (sc *SmartContract) hasExistKeyID(eco, id int64) error {
	key := &sqldb.Key{}
	found, err := key.SetTablePrefix(eco).Get(sc.DbTransaction, id)
	if err != nil {
		return err
	}
	if !found {
		_, _, err = DBInsert(sc, "@1keys", types.LoadMap(map[string]any{
			"id":        id,
			"account":   converter.IDToAddress(id),
			"ecosystem": eco,
		}))
		if err != nil {
			return err
		}
	}
	keyOne := &sqldb.Key{}
	if foundOne, err := keyOne.SetTablePrefix(1).Get(sc.DbTransaction, id); err != nil {
		return err
	} else if !foundOne {
		_, _, err = DBInsert(sc, "@1keys", types.LoadMap(map[string]any{
			"id":        id,
			"account":   converter.IDToAddress(id),
			"ecosystem": 1,
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
			ecosystemWallet := converter.AddressToID(ew.Value)
			if ecosystemWallet != 0 {
				paymentType = PaymentType_EcosystemAddress
				fromID = ecosystemWallet
			}
		}
	}

	return fromID, paymentType
}

// getChangeAddress return the payment address associated with the ecosystem
func (sc *SmartContract) getChangeAddress(eco int64) ([]*PaymentInfo, error) {
	var (
		err         error
		storageFee  decimal.Decimal
		expediteFee decimal.Decimal
		pays        []*PaymentInfo
		feeMode     *sqldb.FeeModeInfo
		curPay      = &PaymentInfo{
			TokenEco:       eco,
			ToID:           sc.BlockHeader.KeyId,
			PayWallet:      new(sqldb.Key),
			Ecosystem:      new(sqldb.Ecosystem),
			Combustion:     new(Combustion),
			FuelCategories: make([]FuelCategory, 0),
			TaxesSize:      syspar.SysInt64(syspar.TaxesSize),
		}
	)
	if _, err = curPay.Ecosystem.Get(sc.DbTransaction, curPay.TokenEco); err != nil {
		return nil, err
	}
	feeMode, err = curPay.Ecosystem.FeeMode()
	if err != nil {
		return nil, err
	}
	if feeMode == nil && eco != consts.DefaultTokenEcosystem {
		return nil, nil
	}

	var f2 float64
	if feeMode != nil {
		f2 = feeMode.FollowFuel
	}
	if curPay.FuelRate, err = sc.fuelRate(curPay.TokenEco, f2); err != nil {
		return nil, err
	}

	if curPay.TaxesID, err = sc.taxesWallet(curPay.TokenEco); err != nil {
		return nil, err
	}

	if expediteFee, err = expediteFeeBy(sc.TxSmart.Expedite, int32(curPay.Ecosystem.Digits)); err != nil {
		return nil, err
	}

	storageFee = storageFeeBy(sc.TxSize, int32(curPay.Ecosystem.Digits))

	curPay.FromID, curPay.PaymentType = sc.getFromIdAndPayType(curPay.TokenEco)
	if feeMode != nil && eco != consts.DefaultTokenEcosystem {
		// only eco > 1
		curPay.Combustion = newCombustion(feeMode.Combustion.Flag, feeMode.Combustion.Percent)
	}

	if feeMode != nil && curPay.TokenEco != consts.DefaultTokenEcosystem {
		if curPay.PaymentType == PaymentType_ContractCaller {
			return nil, nil
		}
		keys := make([]string, 0, len(feeMode.FeeModeDetail))
		for k := range feeMode.FeeModeDetail {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var indirectBool bool
		for i := 0; i < len(keys); i++ {
			k := keys[i]
			flag := feeMode.FeeModeDetail[k]
			if _, ok := FuelType_value[k]; ok && GasPayAbleType(flag.FlagToInt()) == GasPayAbleType_Capable {
				indirectBool = true
				break
			}
		}
		if !indirectBool {
			return nil, nil
		}
		// indirect to reward and taxes for other eco
		indirectPay := &PaymentInfo{
			Indirect:       true,
			TokenEco:       curPay.TokenEco,
			Ecosystem:      curPay.Ecosystem,
			ToID:           curPay.FromID,
			FromID:         sc.TxSmart.KeyID,
			PaymentType:    PaymentType_ContractCaller,
			FuelRate:       curPay.FuelRate,
			FuelCategories: make([]FuelCategory, 0),
			PayWallet:      new(sqldb.Key),
			Combustion:     new(Combustion),
		}
		// caller to reward and taxes for platform eco
		cpyPlatCaller := sc.resetFromIDForNativePay(sc.TxSmart.KeyID)
		cpyPlatCaller.PaymentType = PaymentType_ContractCaller

		// indirect to reward and taxes for platform eco
		cpyPlatIndirect := sc.resetFromIDForNativePay(curPay.FromID)
		cpyPlatIndirect.PaymentType = curPay.PaymentType

		curPay.FromID = sc.TxSmart.KeyID
		curPay.PaymentType = PaymentType_ContractCaller

		// reset sc multiPays
		sc.multiPays = sc.multiPays[:0]

		for i := 0; i < len(keys); i++ {
			k := keys[i]
			flag := feeMode.FeeModeDetail[k]
			var categoryFee decimal.Decimal
			switch k {
			case FuelType_vmCost_fee.String():
				categoryFee = decimal.NewFromInt(0)
			case FuelType_storage_fee.String():
				categoryFee = storageFee
			case FuelType_expedite_fee.String():
				categoryFee = expediteFee
			default:
				continue
			}
			category := NewFuelCategory(FuelType(FuelType_value[k]), categoryFee, GasPayAbleType(flag.FlagToInt()), flag.ConversionRateToFloat())

			switch category.Flag {
			case GasPayAbleType_Unable:
				category.writeDecimal(category.Decimal.Shift(int32(cpyPlatCaller.Ecosystem.Digits - curPay.Ecosystem.Digits)))
				cpyPlatCaller.PushFuelCategories(category)
			case GasPayAbleType_Capable:
				// exclude FuelType_expedite_fee
				if category.FuelType != FuelType_expedite_fee {
					curPay.PushFuelCategories(category)
					category.writeArithmetic(Arithmetic_MUL)
				}
				indirectPay.PushFuelCategories(category)
				category.resetArithmetic()
				category.writeDecimal(category.Decimal.Shift(int32(cpyPlatIndirect.Ecosystem.Digits - curPay.Ecosystem.Digits)))
				if category.FuelType == FuelType_expedite_fee {
					category.writeDecimal(category.Decimal.Div(decimal.NewFromFloat(feeMode.FollowFuel)))
				}
				cpyPlatIndirect.PushFuelCategories(category)
			}
		}
		pays = append(pays, cpyPlatCaller, cpyPlatIndirect, indirectPay, curPay)
		return pays, nil
	}

	curPay.PushFuelCategories(
		NewFuelCategory(FuelType_vmCost_fee, decimal.NewFromInt(0), GasPayAbleType_Unable, 100),
		NewFuelCategory(FuelType_storage_fee, storageFee, GasPayAbleType_Unable, 100),
		NewFuelCategory(FuelType_expedite_fee, expediteFee, GasPayAbleType_Unable, 100),
	)
	pays = append(pays, curPay)
	return pays, nil
}

func (sc *SmartContract) prepareMultiPay() error {
	ownerInfo := sc.TxContract.Info().Owner
	if err := sc.appendTokens(ownerInfo.TokenID, sc.TxSmart.EcosystemID); err != nil {
		return err
	}

	for i := 0; i < len(sc.getTokenEcos()); i++ {
		eco := sc.getTokenEcos()[i]
		pays, err := sc.getChangeAddress(eco)
		if err != nil {
			return err
		}
		for _, pay := range pays {
			if err = pay.checkVerify(sc); err != nil {
				return err
			}
		}
		sc.multiPays = append(sc.multiPays, pays...)
	}
	return nil
}

func (sc *SmartContract) appendTokens(nums ...int64) error {
	sc.TokenEcosystems = make(map[int64]any)
	sc.TokenEcosystems[consts.DefaultTokenEcosystem] = nil

	for i := 0; i < len(nums); i++ {
		num := nums[i]
		if num <= 1 {
			continue
		}
		if _, ok := sc.TokenEcosystems[num]; ok {
			continue
		}
		ecosystems := &sqldb.Ecosystem{}
		_, err := ecosystems.Get(sc.DbTransaction, num)
		if err != nil {
			return err
		}
		if len(ecosystems.TokenSymbol) <= 0 {
			continue
		}
		sc.TokenEcosystems[num] = nil
	}
	return nil
}

func (sc *SmartContract) getTokenEcos() []int64 {
	var ecos []int64
	for i := range sc.TokenEcosystems {
		ecos = append(ecos, i)
	}
	sortkeys.Int64s(ecos)
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

func (sc *SmartContract) fuelRate(eco int64, followFuel float64) (decimal.Decimal, error) {
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
		times := decimal.NewFromFloat(followFuel)
		if times.LessThanOrEqual(zero) {
			times = decimal.New(1, 0)
		}
		follow = follow.Mul(times)

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

func (sc *SmartContract) taxesWallet(eco int64) (taxesID int64, err error) {
	if _, ok := syspar.HasTaxesWallet(eco); !ok {
		var taxesPub []byte
		err = sqldb.GetDB(sc.DbTransaction).Select("pub").
			Model(&sqldb.Key{}).Where("id = ? AND ecosystem = 1",
			syspar.GetTaxesWallet(1)).Row().Scan(&taxesPub)
		if err != nil {
			return
		}
		id := PubToID(fmt.Sprintf("%x", taxesPub))
		if err = sc.hasExistKeyID(eco, id); err != nil {
			return
		}

		taxes := make([][]string, 0)
		err = json.Unmarshal([]byte(syspar.SysString(syspar.TaxesWallet)), &taxes)
		if err != nil {
			return
		}
		var newTaxes []string
		var tax []byte
		newTaxes = append(newTaxes, strconv.FormatInt(eco, 10), strconv.FormatInt(id, 10))
		taxes = append(taxes, newTaxes)
		tax, err = json.Marshal(taxes)
		if err != nil {
			return
		}
		sc.taxes = true
		_, err = UpdatePlatformParam(sc, syspar.TaxesWallet, string(tax), "")
		if err != nil {
			return
		}
	}
	taxesID = converter.StrToInt64(syspar.GetTaxesWallet(eco))
	if taxesID == 0 {
		err = fmt.Errorf("get eco[%d] taxes wallet err", eco)
	}
	return
}

func storageFeeBy(txSize int64, digits int32) decimal.Decimal {
	var (
		storageFee decimal.Decimal
		zero       = decimal.Zero
	)
	storageFee = decimal.NewFromInt(syspar.SysInt64(syspar.PriceTxSize)).
		Mul(decimal.New(1, digits)).
		Mul(decimal.NewFromInt(txSize)).
		Div(decimal.NewFromInt(consts.ChainSize))
	if storageFee.LessThanOrEqual(zero) {
		storageFee = decimal.New(1, 0)
	}
	return storageFee
}

func expediteFeeBy(expedite string, digits int32) (decimal.Decimal, error) {
	zero := decimal.Zero
	if len(expedite) > 0 {
		expedite, err := decimal.NewFromString(expedite)
		if err != nil {
			return zero, fmt.Errorf("expediteFeeBy: %s", err)
		}
		if expedite.LessThan(zero) {
			return zero, fmt.Errorf(eGreaterThan, expedite)
		}
		return expedite.Shift(digits), nil
	}
	return zero, nil
}
