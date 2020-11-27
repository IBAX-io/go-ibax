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

// CheckSum calculates the 0-9 check sum of []byte
func checkSum(val []byte) int {
	var one, two int
	for i, ch := range val {
		digit := int(ch - '0')
		if i&1 == 1 {
			one += digit
		} else {
			two += digit
		}
	}
	checksum := (two + 3*one) % 10
	if checksum > 0 {
		checksum = 10 - checksum
	}
	return checksum
}
