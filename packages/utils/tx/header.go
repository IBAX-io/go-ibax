/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
type Header struct {
	ID          int
	Time        int64
	EcosystemID int64
	KeyID       int64
	NetworkID   int64
	PublicKey   []byte
	//
	//Add sub node processing
	PrivateFor []string
}
