/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package types

// Transaction types.
const (
	FirstBlockTxType = iota + 1
	StopNetworkTxType
	SmartContractTxType
)

type TxHeader struct {
	Type  byte
	Time  uint32
	KeyID int64
}
