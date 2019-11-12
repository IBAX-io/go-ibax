/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
)

// AppParam is model
type AppParam struct {
	ecosystem  int64
	ID         int64  `gorm:"primary_key;not null"`
	AppID      int64  `gorm:"not null"`
	Name       string `gorm:"not null;size:100"`
	Value      string `gorm:"not null"`
	Conditions string `gorm:"not null"`
}

// TableName returns name of table
func (sp *AppParam) TableName() string {
	if sp.ecosystem == 0 {
		sp.ecosystem = 1
	}
	return `1_app_params`
}

// SetTablePrefix is setting table prefix
func (sp *AppParam) SetTablePrefix(tablePrefix string) {
	sp.ecosystem = converter.StrToInt64(tablePrefix)
}

// Get is retrieving model from database
func (sp *AppParam) Get(transaction *DbTransaction, app int64, name string) (bool, error) {
	return isFound(GetDB(transaction).Where("ecosystem=? and app_id=? and name = ?",
		sp.ecosystem, app, name).First(sp))
}

// GetAllAppParameters is returning all state parameters
func (sp *AppParam) GetAllAppParameters(app int64) ([]AppParam, error) {
	parameters := make([]AppParam, 0)
	err := DBConn.Table(sp.TableName()).Where(`ecosystem = ?`, sp.ecosystem).Where(`app_id = ?`, app).Find(&parameters).Error
	if err != nil {
		return nil, err
	}
	return parameters, nil
}

// Get is retrieving model from database
func (sp *AppParam) GetHvlvebalance(transaction *DbTransaction, blockid int64) (decimal.Decimal, error) {

	//var halve,balance string
	ret := decimal.NewFromFloat(0)
	var halve, mine_reward AppParam
	hf, err := isFound(GetDB(transaction).Where("ecosystem=? and app_id=? and name = ?", 1, 1, "halve_interval_blockid").First(&halve))
	if err != nil {
		return ret, err
	}

	md, err := isFound(GetDB(transaction).Where("ecosystem=? and app_id=? and name = ?", 1, 1, "mine_reward").First(&mine_reward))
	if err != nil {
		return ret, err
	}

	if !hf || !md {
		return ret, errors.New("param mine_reward or halve_interval_blockid not found")
	}

	hal := converter.StrToInt64(halve.Value)
	if hal > 0 {
		he := blockid / hal
		mdv := converter.StrToFloat64(mine_reward.Value)

		hm := math.Pow(2, float64(he))
		ret1 := mdv / hm
		ret2 := ret1 / 1000000000000
		ret3 := math.Floor(ret2) * 1000000000000
		ret = decimal.NewFromFloat(ret3)
		return ret, nil
	} else {
		return ret, errors.New("param mine_reward or halve_interval_blockid not ok")
	}
}

// Get is retrieving model from database
func (sp *AppParam) GetFoundationbalance(transaction *DbTransaction) (decimal.Decimal, error) {

	//var halve,balance string
	ret := decimal.NewFromFloat(0)
	var bal AppParam
	hf, err := isFound(GetDB(transaction).Where("ecosystem=? and app_id=? and name = ?", 1, 1, "foundation_reward").First(&bal))
	if err != nil {
		return ret, err
	}
	if !hf {
		return ret, errors.New("param  foundation_reward not found")
	}

	return decimal.NewFromString(bal.Value)
}
