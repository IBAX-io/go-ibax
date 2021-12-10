/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package types

// Header is contain header data
type Header struct {
	ID          int
	Time        int64
	EcosystemID int64
	KeyID       int64
	NetworkID   int64
	PublicKey   []byte
}

// SmartContract is storing smart contract data
type SmartContract struct {
	*Header
	TokenEcosystems map[int64]interface{}
	MaxSum          string
	PayOver         string
	Lang            string
	Expedite        string
	SignedBy        int64
	Params          map[string]interface{}
}
