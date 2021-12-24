/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"errors"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/consts"
)

// TODO In order to add new crypto provider with another key length it will be neccecary to fix constant blocksizes like
// crypto func getSharedKey() pub.X = new(big.Int).SetBytes(public[0:32])
// egcons func checkKey() gSettings.Key = hex.EncodeToString(privKey[aes.BlockSize:])

type cryptoProvider int
type ellipticSizeProvider int

const (
	_AESCBC cryptoProvider = iota
)

const (
	elliptic256 ellipticSizeProvider = iota
)

var (
	// ErrHashing is Hashing error
	ErrHashing = errors.New("Hashing error") // nolint
	// ErrEncrypting is Encoding error
	ErrEncrypting = errors.New("Encoding error")
	// ErrDecrypting is Decrypting error
	ErrDecrypting = errors.New("Decrypting error")
	// ErrUnknownProvider is Unknown provider error
	ErrUnknownProvider = errors.New("Unknown provider")
	// ErrHashingEmpty is Hashing empty value error
	ErrHashingEmpty = errors.New("Hashing empty value")
	// ErrEncryptingEmpty is Encrypting empty value error
	ErrEncryptingEmpty = errors.New("Encrypting empty value")
	// ErrDecryptingEmpty is Decrypting empty value error
	ErrDecryptingEmpty = errors.New("Decrypting empty value")
	// ErrSigningEmpty is Signing empty value error
	ErrSigningEmpty = errors.New("Signing empty value")
	// ErrCheckingSignEmpty is Checking sign of empty error
	ErrCheckingSignEmpty = errors.New("Cheking sign of empty")
	// ErrIncorrectSign is Incorrect sign
	ErrIncorrectSign = errors.New("Incorrect sign")
	// ErrUnsupportedCurveSize is Unsupported curve size error
	ErrUnsupportedCurveSize = errors.New("Unsupported curve size")
	// ErrIncorrectPrivKeyLength is Incorrect private key length error
	ErrIncorrectPrivKeyLength = errors.New("Incorrect private key length")
	// ErrIncorrectPubKeyLength is Incorrect public key length
	ErrIncorrectPubKeyLength = errors.New("Incorrect public key length")
)

var (
	cryptoProv   = _AESCBC
	hashProv     = _SHA256
	ellipticSize = elliptic256
	signProv     = _ECDSA
	checksumProv = _CRC64
	hmacProv     = _SHA256
)

// Encrypt is encrypting
func Encrypt(msg []byte, key []byte, iv []byte) ([]byte, error) {
	if len(msg) == 0 {
		return nil, ErrEncryptingEmpty
	}
	switch cryptoProv {
	case _AESCBC:
		return encryptCBC(msg, key, iv)
	default:
		return nil, ErrUnknownProvider
	}
}

// Decrypt is decrypting
func Decrypt(msg []byte, key []byte, iv []byte) ([]byte, error) {
	if len(msg) == 0 {
		return nil, ErrDecryptingEmpty
	}
	switch cryptoProv {
	case _AESCBC:
		return decryptCBC(msg, key, iv)
	default:
		return nil, ErrUnknownProvider
	}
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
	plaintext := _PKCS7Padding(text, consts.BlockSize)
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
	if ret, err = _PKCS7UnPadding(ret); err != nil {
		return nil, err
	}
	return ret, nil

}

// PKCS7Padding realizes PKCS#7 encoding which is described in RFC 5652.
func _PKCS7Padding(src []byte, blockSize int) []byte {
	padding := blockSize - len(src)%blockSize
	return append(src, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

// PKCS7UnPadding realizes PKCS#7 decoding.
func _PKCS7UnPadding(src []byte) ([]byte, error) {
	length := len(src)
	padLength := int(src[length-1])
	for i := length - padLength; i < length; i++ {
		if int(src[i]) != padLength {
			return nil, fmt.Errorf(`incorrect input of PKCS7UnPadding`)
		}
	}
	return src[:length-int(src[length-1])], nil

}
