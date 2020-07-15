/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package model

import (
	"errors"
)

type MineOwner struct {
	ID          int64 `gorm:"not null"`
	Devid       int64 `gorm:"primary_key;not null"`
	Keyid       int64 `gorm:"not null"`
	Minerid     int64 `gorm:"not null"`
	Type        int64 `gorm:"not null"`
	Transfers   int64 `gorm:"not null"`
	Deleted     int64 `gorm:"not null"`
	DateDeleted int64 `gorm:"not null"`
	DateUpdated int64 `gorm:"not null"`
	DateCreated int64 `gorm:"not null"`
}

// TableName returns name of table
func (m MineOwner) TableName() string {
	return `1_mine_owner`
}

// Get is retrieving model from database
func (m *MineOwner) GetPoolManage(keyid int64) (bool, error) {

	var k Key
	d := k.SetTablePrefix(1)
	f1, err1 := d.Get(nil, keyid)
	if err1 != nil {
		return false, err1
	}

	if !f1 {
		return f1, errors.New("key not found")
	}

	var mo MineOwner
	fo, erro := mo.GetPool(keyid)
	if erro != nil {
		return false, erro
	}

	if !fo {

		var mh MintPoolTransferHistory
		fh, erro := mh.GetPool(keyid)
		if erro != nil {
			return false, erro
		}

		if fh {
			return true, nil
		}

		var mr MinePoolApplyInfo
		fr, erro := mr.GetPool(keyid)
		if erro != nil {
			return false, erro
		}

		if fr {
			return true, nil
		}

		return fh, errors.New("MintPoolTransferHistory keyid not found")
	}

	return true, nil

}

// Get is retrieving model from database
func (m *MineOwner) GetAllPoolManage(dbt *DbTransaction, ts int64) (map[int64]int64, error) {
	var mp []MineOwner
	ret := make(map[int64]int64)
	//DBConn.Table(m.TableName()).Where("etime <=?", time).Delete(MineInvitepowadd{})
	err := GetDB(dbt).Table(m.TableName()).
		Where("date_created <= ? and deleted =? and type = ?", ts, 0, 2).
		Order("devid asc").
		Find(&mp).Error
	if err != nil {
		return ret, err
	}

	for _, v := range mp {
		ret[v.Devid] = v.Minerid
	}

	return ret, err
}
