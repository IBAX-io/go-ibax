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
			return ret, errors.New("foundation_balance not found")
		}
		var ap AppParam
		fa, err := ap.GetFoundationbalance(transaction)
		if err != nil {
			return ret, err
		}
		ret, err = decimal.NewFromString(sp.Value)
		if err != nil {
			return ret, err
		}
		if fa.LessThanOrEqual(ret) {
			return fa, nil
		} else {
			return ret, nil
		}

	} else {
		return ret, errors.New("mineowner not found")
	}

}
