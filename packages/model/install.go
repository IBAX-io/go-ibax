/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package model

// ProgressComplete status of installation progress
const ProgressComplete = "complete"

// Install is model
type Install struct {
// Create is creating record of model
func (i *Install) Create() error {
	return DBConn.Create(i).Error
}
