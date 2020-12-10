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

		return
	}

	tpass, err := AesDecrypt(bytesPass, aeskey)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("aesdecrypt:%s\n", tpass)
}
