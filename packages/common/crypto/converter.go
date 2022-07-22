/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package crypto

import (
	"crypto/sha512"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
)

// CutPub removes the first 04 byte
func CutPub(pubKey []byte) []byte {
	if len(pubKey) == 65 && pubKey[0] == 4 {
		pubKey = pubKey[1:]
	}
	return pubKey
}

// Address gets int64 address from the public key
func Address(pubKey []byte) int64 {
	pubKey = CutPub(pubKey)
	h := Hash(pubKey)
	h512 := sha512.Sum512(h[:])
	crc := CalcChecksum(h512[:])
	return buildChecksumConvert(crc)
}

func buildChecksumConvert(crc uint64) int64 {
	num := strconv.FormatUint(crc, 10)
	val := RepeatPrefixed(num)
	v := val[:len(val)-1]
	sum := converter.CheckSum(v)
	uSum := uint64(sum)
	return int64(crc - (crc % 10) + uSum)
}

func RepeatPrefixed(input string) []byte {
	const size = consts.AddressLength
	if len(input) > size {
		input = input[:size]
	}
	return []byte(strings.Repeat("0", size-len(input)) + input)
}

// KeyToAddress converts a public key to chain address XXXX-...-XXXX.
func KeyToAddress(pubKey []byte) string {
	return converter.AddressToString(Address(pubKey))
}

// GetWalletIDByPublicKey converts public key to wallet id
func GetWalletIDByPublicKey(publicKey []byte) (int64, error) {
	key, _ := HexToPub(string(publicKey))
	return int64(Address(key)), nil
}

// HexToPub encodes hex string to []byte of pub key
func HexToPub(pub string) ([]byte, error) {
	key, err := hex.DecodeString(pub)
	if err != nil {
		return nil, err
	}
	return CutPub(key), nil
}

// PubToHex decodes []byte of pub key to hex string
func PubToHex(pub []byte) string {
	if len(pub) == 64 {
		pub = append([]byte{4}, pub...)
	}
	return hex.EncodeToString(pub)
}
