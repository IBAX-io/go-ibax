package model

import (
	"encoding/json"
	"errors"
	"github.com/shopspring/decimal"
	//"time"
)

type AssignRules struct {
	StartBlockID    int64  `json:"start_blockid"`
	EndBlockID      int64  `json:"end_blockid"`
	IntervalBlockID int64  `json:"interval_blockid"`
	Count           int64  `json:"count"`
	TotalAmount     string `json:"total_amount"`
}

// AssignGetInfo is model
type AssignGetInfo struct {
	ID            int64           `gorm:"primary_key;not null"`
	Type          int64           `gorm:"not null"`
	Keyid         int64           `gorm:"not null"`
	TotalAmount   decimal.Decimal `gorm:"not null"`
	BalanceAmount decimal.Decimal `gorm:"not null"`
	Amount        decimal.Decimal `gorm:"not null"`
	Latestid      int64           `gorm:"not null"`
	Deleted       int64           `gorm:"not null"`
	DateUpdated   int64           `gorm:"not null" `
	DateCreated   int64           `gorm:"not null" `
}

// TableName returns name of table
func (m AssignGetInfo) TableName() string {
	return `1_assign_get_info`
}

// Get is retrieving model from database
func (m *AssignGetInfo) GetBalance(db *DbTransaction, wallet int64) (bool, decimal.Decimal, decimal.Decimal, error) {

	var mps []AssignGetInfo
	var balance, total_balance decimal.Decimal
	balance = decimal.NewFromFloat(0)
	total_balance = decimal.NewFromFloat(0)
	err := GetDB(db).Table(m.TableName()).
		Where("keyid = ? and deleted =? ", wallet, 0).
		Find(&mps).Error
	if err != nil {
		return false, balance, total_balance, err
	}
	if len(mps) == 0 {
		return false, balance, total_balance, err
	}

	//newblockid
	block := &Block{}
	found, err := block.GetMaxBlock()
	if err != nil {
		return false, balance, total_balance, err
	}
	if !found {
		return false, balance, total_balance, errors.New("maxblockid not found")
	}

	//assign_rule
	var sp StateParameter
	sp.SetTablePrefix(`1`)
	found1, err1 := sp.Get(db, `assign_rule`)
	if err1 != nil {
		return false, balance, total_balance, err1
	}

	if !found1 || len(sp.Value) == 0 {
		return false, balance, total_balance, errors.New("assign_rule not found or not exist assign_rule")
	}
	for _, t := range mps {
		am := decimal.NewFromFloat(0)
		tm := t.BalanceAmount
		rule, ok := rules[t.Type]
		if ok {
			sid := rule.StartBlockID
			iid := rule.IntervalBlockID
			eid := rule.EndBlockID

			if maxblockid >= eid {
				am = tm
			} else {
				if t.Latestid == 0 {
					count := int64(0)
					if maxblockid > sid {
						count = (maxblockid - sid) / iid
						count += 1
					}
					if count > 0 {
						if t.Type == 4 {
							//first
							if t.Latestid == 0 {
								am = t.BalanceAmount.Mul(decimal.NewFromFloat(0.1))
								sm := t.Amount.Mul(decimal.NewFromFloat(float64(count - 1)))
								am = am.Add(sm)
							} else {
								am = t.Amount.Mul(decimal.NewFromFloat(float64(count)))
							}

						} else {
							am = t.Amount.Mul(decimal.NewFromFloat(float64(count)))
						}
					}

				} else {
					if maxblockid > t.Latestid {
						count := (maxblockid - t.Latestid) / iid
						am = t.Amount.Mul(decimal.NewFromFloat(float64(count)))
					}
				}
			}

			tm = tm.Sub(am)
			balance = balance.Add(am)
			total_balance = total_balance.Add(tm)
		}
	}
	return true, balance, total_balance, err
}
