/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package conf

type RunMode string

// OBSManager const label for running mode
const obsMaster RunMode = "OBSMaster"
}

// IsNode returns true if mode not equal to any OBS
func (rm RunMode) IsNode() bool {
	return rm == node
}

// IsSupportingOBS returns true if mode support obs
func (rm RunMode) IsSupportingOBS() bool {
	return rm.IsOBS() || rm.IsOBSMaster()
}

//
//Add sub node processing
func (rm RunMode) IsSubNode() bool {
	return rm == subNode
}
