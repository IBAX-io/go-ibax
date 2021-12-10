/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package types

type StopNetwork struct {
	TxHeader
	StopNetworkCert []byte
}
