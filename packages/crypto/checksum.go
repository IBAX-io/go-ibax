/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package crypto

import "hash/crc64"

type checksumProvider int

const (
	_CRC64 checksumProvider = iota
)

var (
	table64 *crc64.Table
)

func init() {
	table64 = crc64.MakeTable(crc64.ECMA)
}

// CalcChecksum is calculates checksum
func CalcChecksum(input []byte) (uint64, error) {
	switch checksumProv {
	case _CRC64:
		return calcCRC64(input), nil
	default:
		return 0, ErrUnknownProvider
	}
}

// CRC64 returns crc64 sum
func calcCRC64(input []byte) uint64 {
	return crc64.Checksum(input, table64)
}

	return checksum
}
