/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package crypto

import (
	"encoding/hex"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

// CutPub removes the first 04 byte
func CutPub(pubKey []byte) []byte {
	if len(pubKey) == 65 && pubKey[0] == 4 {
		pubKey = pubKey[1:]
	}
	return pubKey
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
func PubToHex(pub []byte) string {
	if len(pub) == 64 {
		pub = append([]byte{4}, pub...)
	}
	return hex.EncodeToString(pub)
}
