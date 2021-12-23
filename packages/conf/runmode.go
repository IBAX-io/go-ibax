/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package conf

type RunMode string

const (
	// running mode
	node      RunMode = "NONE"
	clbMaster RunMode = "CLBMaster"
	clb       RunMode = "CLB"
	subNode   RunMode = "SubNode"
)

// IsCLBMaster returns true if mode equal clbMaster
func (rm RunMode) IsCLBMaster() bool {
	return rm == clbMaster
}

// IsCLB returns true if mode equal clb
func (rm RunMode) IsCLB() bool {
	return rm == clb
}

// IsNode returns true if mode not equal to any CLB
func (rm RunMode) IsNode() bool {
	return rm == node
}

// IsSupportingCLB returns true if mode support clb
func (rm RunMode) IsSupportingCLB() bool {
	return rm.IsCLB() || rm.IsCLBMaster()
}

func (rm RunMode) IsSubNode() bool {
	return rm == subNode
}

// IsCLB check running mode
func (c GlobalConfig) IsCLB() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsCLB()
}

// IsCLBMaster check running mode
func (c GlobalConfig) IsCLBMaster() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsCLBMaster()
}

// IsSupportingCLB check running mode
func (c GlobalConfig) IsSupportingCLB() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsSupportingCLB()
}

// IsNode check running mode
func (c GlobalConfig) IsNode() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsNode()
}

// IsSubNode check running mode
func (c GlobalConfig) IsSubNode() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsSubNode()
}
