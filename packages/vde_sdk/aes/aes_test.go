/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package aes

import (
	"encoding/base64"
	"fmt"
	"testing"
)

func TestAesEncryptAndDecrypt(t *testing.T) {

	var aeskey = []byte("123456789012345612345678") //AES-128(16bytes)AES-256(32bytes)
	pass := []byte("This is my private data!")
	fmt.Printf("password:%v\n", string(aeskey))
	if err != nil {
		fmt.Println(err)
		return
	}

	tpass, err := AesDecrypt(bytesPass, aeskey)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("aesdecrypto:%s\n", tpass)
}
