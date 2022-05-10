/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

// Role is model
type Role struct {
	ecosystem   int64
	ID          int64  `gorm:"primary_key;not null" json:"id"`
	DefaultPage string `gorm:"not null" json:"default_page"`
	RoleName    string `gorm:"not null" json:"role_name"`
	Deleted     int64  `gorm:"not null" json:"deleted"`
	RoleType    int64  `gorm:"not null" json:"role_type"`
}

// SetTablePrefix is setting table prefix
func (r *Role) SetTablePrefix(prefix int64) {
	r.ecosystem = prefix
}

// TableName returns name of table
func (r *Role) TableName() string {
	if r.ecosystem == 0 {
		r.ecosystem = 1
	}
	return "1_roles"
}

// Get is retrieving model from database
func (r *Role) Get(dbTx *DbTransaction, id int64) (bool, error) {
	return isFound(GetDB(dbTx).First(&r, id))
}
