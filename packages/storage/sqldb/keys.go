/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

// Key is model
type Key struct {
	ecosystem    int64
	accountKeyID int64 `gorm:"-"`

	ID        int64  `gorm:"primary_key;not null"`
	AccountID string `gorm:"column:account;not null"`
	PublicKey []byte `gorm:"column:pub;not null"`
	Amount    string `gorm:"not null"`
	Maxpay    string `gorm:"not null"`
	Deleted   int64  `gorm:"not null"`
	Blocked   int64  `gorm:"not null"`
}

// SetTablePrefix is setting table prefix
func (m *Key) SetTablePrefix(prefix int64) *Key {
	m.ecosystem = prefix
	return m
}

// TableName returns name of table
func (m Key) TableName() string {
	if m.ecosystem == 0 {
		m.ecosystem = 1
	}
	return `1_keys`
}
func (m *Key) Disable() bool {
	return m.Deleted != 0 || m.Blocked != 0
}
func (m *Key) CapableAmount() decimal.Decimal {
	amount := decimal.Zero
	if len(m.Amount) > 0 {
		amount, _ = decimal.NewFromString(m.Amount)
	}
	maxpay := decimal.Zero
	if len(m.Maxpay) > 0 {
		maxpay, _ = decimal.NewFromString(m.Maxpay)
	}
	if maxpay.GreaterThan(decimal.Zero) && maxpay.LessThan(amount) {
		amount = maxpay
	}
	return amount
}

// Get is retrieving model from database
func (m *Key) Get(db *DbTransaction, wallet int64) (bool, error) {
	return isFound(GetDB(db).Where("id = ? and ecosystem = ?", wallet, m.ecosystem).First(m))
}

func (m *Key) AccountKeyID() int64 {
	if m.accountKeyID == 0 {
		m.accountKeyID = converter.StringToAddress(m.AccountID)
	}
	return m.accountKeyID
}

// KeyTableName returns name of key table
func KeyTableName(prefix int64) string {
	return fmt.Sprintf("%d_keys", prefix)
}

// GetKeysCount returns common count of keys
func GetKeysCount() (int64, error) {
	var cnt int64
	row := DBConn.Raw(`SELECT count(*) key_count FROM "1_keys" WHERE ecosystem = 1`).Select("key_count").Row()
	err := row.Scan(&cnt)
	return cnt, err
}
