/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

// Signature is model
type Signature struct {

// TableName returns name of table
func (s *Signature) TableName() string {
	return s.tableName
}

// Get is retrieving model from database
func (s *Signature) Get(name string) (bool, error) {
	return isFound(DBConn.Where("name = ?", name).First(s))
}
