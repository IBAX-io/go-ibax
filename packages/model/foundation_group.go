package model

import (
	"errors"
	"github.com/shopspring/decimal"
)

//FoundationGroup example
type FoundationGroup struct {
	Id           int64 `gorm:"not null" example:"1"` //
	Keyid        int64 `gorm:"not null"`
	Deleted      int64 `gorm:"not null" example:"1"`                   //
	Date_updated int64 `gorm:"not null" example:"2019-07-19 17:45:31"` //
	Date_created int64 `gorm:"not null" example:"2019-07-19 17:45:31"` //
}

func (FoundationGroup) TableName() string {
	return `1_foundation_group`
}

// Get is retrieving model from database
func (m *FoundationGroup) GetByKeyid(transaction *DbTransaction, keyid int64) (bool, error) {
	return isFound(GetDB(transaction).Where("keyid = ?", keyid).First(m))
}

// Get is retrieving model from database
func (m *FoundationGroup) GetByDevid(transaction *DbTransaction, devid int64) (decimal.Decimal, error) {
	ret := decimal.NewFromFloat(0)
	var mo MineOwner
	f, err := mo.GetByTransaction(transaction, devid)
	if err != nil {
		return ret, err
	}
	if f {
		fb, err := m.GetByKeyid(transaction, mo.Keyid)
		if err != nil {
			return ret, err
		}
		if !fb {
			return ret, nil
		}

		var sp StateParameter
		sp.ecosystem = 1
		fs, err := sp.Get(transaction, "foundation_balance")
		if err != nil {
			return ret, err
		}
		if !fs {
		}

	} else {
		return ret, errors.New("mineowner not found")
	}

}
