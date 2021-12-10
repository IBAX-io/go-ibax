/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package types

// FirstBlock is the header of FirstBlock transaction
type FirstBlock struct {
	TxHeader
	PublicKey             []byte
	NodePublicKey         []byte
	StopNetworkCertBundle []byte
	Test                  int64
	PrivateBlockchain     uint64
}
