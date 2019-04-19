/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

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
func (r *Role) Get(transaction *DbTransaction, id int64) (bool, error) {
	return isFound(GetDB(transaction).First(&r, id))
}
