/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package crypto

import "hash/crc64"

var (
	table64 *crc64.Table
)

func init() {
	table64 = crc64.MakeTable(crc64.ECMA)
}

// CalcChecksum is calculates checksum, returns crc64 sum.
func CalcChecksum(input []byte) uint64 {
	return crc64.Checksum(input, table64)
}
