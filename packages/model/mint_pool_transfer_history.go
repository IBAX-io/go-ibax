package model

type MintPoolTransferHistory struct {
	ID                       int64  `gorm:"not null"`
	Poolid                   int64  `gorm:"not null"`
	Keyid                    int64  `gorm:"not null"`
	return `1_mint_pool_transfer_history`
}

// Get is retrieving model from database
func (m *MintPoolTransferHistory) Get(devid int64) (bool, error) {
	return isFound(DBConn.Where("devid = ?", devid).First(m))
}

// Get is retrieving model from database
func (m *MintPoolTransferHistory) GetPool(keyid int64) (bool, error) {
	return isFound(DBConn.Where("keyid = ? and  status = ?", keyid, 1).Last(m))
}

// Get is retrieving model from database
func (m *MintPoolTransferHistory) GetPoolTransfer(poolid, keyid int64) (bool, error) {
	return isFound(DBConn.Where("poolid = ? and keyid = ? and  status = ?", poolid, keyid, 1).Last(m))
}
