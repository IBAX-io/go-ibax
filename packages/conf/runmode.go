/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package conf

type RunMode string

const (
	// running mode
	node       RunMode = "NONE"
	chainHost  RunMode = "ChainHost"
	childChain RunMode = "ChildChain"
	subNode    RunMode = "SubNode"
)

// IsChainHost returns true if mode equal chainHost
//
// NOTE (2026-10): renamed from CLBMaster. The old name collided with "CLB"
// (Cross Ledger Base), the cross-ledger communication protocol implemented by
// the separate go-ibax-clb repository. This run mode only manages isolated
// child-chain instances and has no cross-ledger function.
func (rm RunMode) IsChainHost() bool {
	return rm == chainHost
}

// IsChildChain returns true if mode equal childChain
//
// NOTE (2026-10): renamed from CLB (run mode). See IsChainHost note.
func (rm RunMode) IsChildChain() bool {
	return rm == childChain
}

// IsNode returns true if mode is a regular node (not a child-chain mode)
func (rm RunMode) IsNode() bool {
	return rm == node
}

// IsSupportingChildChain returns true if mode supports child chains
func (rm RunMode) IsSupportingChildChain() bool {
	return rm.IsChildChain() || rm.IsChainHost()
}

func (rm RunMode) IsSubNode() bool {
	return rm == subNode
}

// IsChildChain check running mode
func (c GlobalConfig) IsChildChain() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsChildChain()
}

// IsChainHost check running mode
func (c GlobalConfig) IsChainHost() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsChainHost()
}

// IsSupportingChildChain check running mode
func (c GlobalConfig) IsSupportingChildChain() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsSupportingChildChain()
}

// IsNode check running mode
func (c GlobalConfig) IsNode() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsNode()
}

// IsSubNode check running mode
func (c GlobalConfig) IsSubNode() bool {
	return RunMode(c.LocalConf.RunNodeMode).IsSubNode()
}

// Deprecated: use IsChainHost. Kept for backward compatibility during transition.
func (rm RunMode) IsCLBMaster() bool {
	return rm.IsChainHost()
}

// Deprecated: use IsChildChain. Kept for backward compatibility during transition.
func (rm RunMode) IsCLB() bool {
	return rm.IsChildChain()
}

// Deprecated: use IsSupportingChildChain. Kept for backward compatibility during transition.
func (rm RunMode) IsSupportingCLB() bool {
	return rm.IsSupportingChildChain()
}

// Deprecated: use GlobalConfig.IsChainHost.
func (c GlobalConfig) IsCLBMaster() bool {
	return c.IsChainHost()
}

// Deprecated: use GlobalConfig.IsChildChain.
func (c GlobalConfig) IsCLB() bool {
	return c.IsChildChain()
}

// Deprecated: use GlobalConfig.IsSupportingChildChain.
func (c GlobalConfig) IsSupportingCLB() bool {
	return c.IsSupportingChildChain()
}
