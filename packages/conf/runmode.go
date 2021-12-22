/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package conf

type RunMode string

const (
	// running mode
	node      RunMode = "NONE"
	obsMaster RunMode = "OBSMaster"
	obs       RunMode = "OBS"
	subNode   RunMode = "SubNode"
)

// IsOBSMaster returns true if mode equal obsMaster
func (rm RunMode) IsOBSMaster() bool {
	return rm == obsMaster
}

// IsOBS returns true if mode equal obs
func (rm RunMode) IsOBS() bool {
	return rm == obs
}

// IsNode returns true if mode not equal to any OBS
func (rm RunMode) IsNode() bool {
	return rm == node
}

// IsSupportingOBS returns true if mode support obs
func (rm RunMode) IsSupportingOBS() bool {
	return rm.IsOBS() || rm.IsOBSMaster()
}

func (rm RunMode) IsSubNode() bool {
	return rm == subNode
}

// IsOBS check running mode
func (c GlobalConfig) IsOBS() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsOBS()
}

// IsOBSMaster check running mode
func (c GlobalConfig) IsOBSMaster() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsOBSMaster()
}

// IsSupportingOBS check running mode
func (c GlobalConfig) IsSupportingOBS() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsSupportingOBS()
}

// IsNode check running mode
func (c GlobalConfig) IsNode() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsNode()
}

// IsSubNode check running mode
func (c GlobalConfig) IsSubNode() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsSubNode()
}
