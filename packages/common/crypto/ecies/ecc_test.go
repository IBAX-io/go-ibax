/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package ecies

import (
	"encoding/hex"
	"fmt"
	"testing"
)

// HexToBytes converts the hexadecimal representation to []byte
func HexToBytes(hexdata string) ([]byte, error) {
	return hex.DecodeString(hexdata)
}

func TestEccencryptoKey(t *testing.T) {
	plainText := []byte("ecc hello")

	privateHex := "5d2275c0888d1576e15a45b7eeb870b26a45ceb89f37e586ee21b07c14b0541a"
	pubkeyHex := "0463fbbfefe076637384717297f9f09951e8a2a02480b14cfbd1ed4050ff07d2882a67212dce487ed5cee93fcc3126e9197b73eea02d2a73c64a4906ece24fad67"

	privateKeyBytes, err := HexToBytes(privateHex)
	if err != nil {
		fmt.Println(err)
	}

	publicKeyBytes, err := crypto.HexToPub(pubkeyHex)
	if err != nil {
		fmt.Println(err)
	}

	pub, err2 := crypto.GetPublicKeys(publicKeyBytes)
	if err2 != nil {
		fmt.Println(err2)
	}

	pri, err2 := crypto.GetPrivateKeys(privateKeyBytes)
	if err2 != nil {
		fmt.Println(err2)
	}

	cryptText, _ := EccPubEncrypt(plainText, pub)
	fmt.Println("ECC：", hex.EncodeToString(cryptText))

	msg, err := EccPriDeCrypt(cryptText, pri)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("ECC：", string(msg))

}
