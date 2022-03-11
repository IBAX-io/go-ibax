/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

// Confirmation is model
type Confirmation struct {
	BlockID int64 `gorm:"primary_key"`
	Good    int32 `gorm:"not null"`
	Bad     int32 `gorm:"not null"`
	Time    int64 `gorm:"not null"`
}

// GetGoodBlock returns last good block
func (c *Confirmation) GetGoodBlock(goodCount int) (bool, error) {
	return isFound(DBConn.Where("good >= ?", goodCount).Last(&c))
}

// GetConfirmation returns if block with blockID exists
func (c *Confirmation) GetConfirmation(blockID int64) (bool, error) {
	return isFound(DBConn.Where("block_id= ?", blockID).First(&c))
}

// Save is saving model
func (c *Confirmation) Save() error {
	return DBConn.Save(c).Error
}

// GetGoodBlockLast returns last good block
func (c *Confirmation) GetGoodBlockLast() (bool, error) {
	var sp PlatformParameter
	count, err := sp.GetNumberOfHonorNodes()
	if err != nil {
		return false, err
	}
	return isFound(DBConn.Where("good >= ?", int(count/2)).Last(&c))
}

// GetGoodBlock returns last good block
func (c *Confirmation) CheckAllowGenBlock() (bool, error) {
	prevBlock := &InfoBlock{}
	_, err := prevBlock.Get()
	if err != nil {
		return false, err
	}

	var sp PlatformParameter
	count, err := sp.GetNumberOfHonorNodes()
	if err != nil {
		return false, err
	}

	if count == 0 {
		return true, nil
	}

	f, err := c.GetGoodBlock(count / 2)
	if err != nil {
		return false, err
	}
	if f {
		if prevBlock.BlockID-c.BlockID < 1 {
			return true, nil
		}
	}

	return false, err
}
