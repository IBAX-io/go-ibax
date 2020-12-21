/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tx

// SmartContract is storing smart contract data
type SmartContract struct {
	Header
	TokenEcosystems map[int64]interface{}
	MaxSum          string
	PayOver         string
	Lang            string
	Expedite        string
	SignedBy        int64
