/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package aes

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/consts"
)

// Encrypt is encrypting
func Encrypt(msg []byte, key []byte, iv []byte) ([]byte, error) {
	return encryptCBC(msg, key, iv)
}

// Decrypt is decrypting
func Decrypt(msg []byte, key []byte, iv []byte) ([]byte, error) {
	return decryptCBC(msg, key, iv)
}

// SharedEncrypt creates a shared key and encrypts text. The first 32 characters are the created public key.
// The cipher text can be only decrypted with the original private key.
//func SharedEncrypt(public, text []byte) ([]byte, error) {
//	priv, pub, err := genBytesKeys()
//	if err != nil {
//		return nil, err
//	}
//	shared, err := getSharedKey(priv, public)
//	if err != nil {
//		return nil, err
//	}
//	val, err := Encrypt(shared, text, pub)
//	return val, err
//}

// CBCEncrypt encrypts the text by using the key parameter. It uses CBC mode of AES.
func encryptCBC(text, key, iv []byte) ([]byte, error) {
	if iv == nil {
		iv = make([]byte, consts.BlockSize)
		if _, err := crand.Read(iv); err != nil {
			return nil, err
		}
	} else if len(iv) < consts.BlockSize {
		return nil, fmt.Errorf(`wrong size of iv %d`, len(iv))
	} else if len(iv) > consts.BlockSize {
		iv = iv[:consts.BlockSize]
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plaintext := pKCS7Padding(text, consts.BlockSize)
	mode := cipher.NewCBCEncrypter(block, iv)
	encrypted := make([]byte, len(plaintext))
	mode.CryptBlocks(encrypted, plaintext)
	return append(iv, encrypted...), nil

}

// CBCDecrypt decrypts the text by using key. It uses CBC mode of AES.
func decryptCBC(ciphertext, key, iv []byte) ([]byte, error) {
	if iv == nil {
		iv = ciphertext[:consts.BlockSize]
		ciphertext = ciphertext[consts.BlockSize:]
	}
	if len(ciphertext) < consts.BlockSize || len(ciphertext)%consts.BlockSize != 0 {
		return nil, fmt.Errorf(`wrong size of cipher %d`, len(ciphertext))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	ret := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv[:consts.BlockSize]).CryptBlocks(ret, ciphertext)
	if ret, err = pKCS7UnPadding(ret); err != nil {
		return nil, err
	}
	return ret, nil

}

// pKCS7Padding realizes PKCS#7 encoding which is described in RFC 5652.
func pKCS7Padding(src []byte, blockSize int) []byte {
	padding := blockSize - len(src)%blockSize
	return append(src, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

// pKCS7UnPadding realizes PKCS#7 decoding.
func pKCS7UnPadding(src []byte) ([]byte, error) {
	length := len(src)
	padLength := int(src[length-1])
	for i := length - padLength; i < length; i++ {
		if int(src[i]) != padLength {
			return nil, fmt.Errorf(`incorrect input of PKCS7UnPadding`)
		}
	}
	return src[:length-int(src[length-1])], nil
}
func PKCS5Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func PKCS5UnPadding(origData []byte) []byte {
	length := len(origData)
	unpadding := int(origData[length-1])
	return origData[:(length - unpadding)]
}

func AesEncrypt(origData, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	origData = PKCS5Padding(origData, blockSize)
	blockMode := cipher.NewCBCEncrypter(block, key[:blockSize])
	crypted := make([]byte, len(origData))
	blockMode.CryptBlocks(crypted, origData)
	return crypted, nil
}

func AesDecrypt(crypted, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	blockSize := block.BlockSize()
	blockMode := cipher.NewCBCDecrypter(block, key[:blockSize])
	origData := make([]byte, len(crypted))
	blockMode.CryptBlocks(origData, crypted)
	origData = PKCS5UnPadding(origData)
	return origData, nil
}
