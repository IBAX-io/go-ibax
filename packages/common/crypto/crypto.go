/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package crypto

import (
	"encoding/hex"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/IBAX-io/go-ibax/packages/common/crypto/asymalgo"
	"github.com/IBAX-io/go-ibax/packages/common/crypto/hashalgo"
)

var (
	asymAlgo AsymAlgo
	hashAlgo HashAlgo
)

func NewAsymAlgo(a AsymAlgo) AsymProvider {
	switch a {
	case AsymAlgo_ECC_P256:
		return &asymalgo.P256{}
	case AsymAlgo_ECC_Secp256k1:
		return &asymalgo.Secp256k1{}
	case AsymAlgo_SM2:
		return &asymalgo.SM2{}
	}
	panic(fmt.Errorf("curve algo [%v] is not supported yet", a))
}

func InitAsymAlgo(s string) {
	v, ok := AsymAlgo_value[s]
	if !ok {
		log.Fatal(fmt.Errorf("curve algo [%v] is not supported yet, Run 'go-ibax config --help' for details", s))
	}
	asymAlgo = AsymAlgo(v)
	return
}

func GetAsymProvider() AsymProvider {
	return NewAsymAlgo(asymAlgo)
}

func NewHashAlgo(a HashAlgo) HashProvider {
	switch a {
	case HashAlgo_SHA256:
		return &hashalgo.SHA256{}
	case HashAlgo_SM3:
		return &hashalgo.SM3{}
	case HashAlgo_KECCAK256:
		return &hashalgo.Keccak256{}
	case HashAlgo_SHA3_256:
		return &hashalgo.Sha3256{}
	}
	panic(fmt.Errorf("hash algo [%v] is not supported yet", a))
}

func InitHashAlgo(s string) {
	v, ok := HashAlgo_value[s]
	if !ok {
		log.Fatal(fmt.Errorf("hash algo [%v] is not supported yet, Run 'go-ibax config --help' for details", s))
	}
	hashAlgo = HashAlgo(v)
	return
}

func GetHashProvider() HashProvider {
	return NewHashAlgo(hashAlgo)
}

// GenKeyPair generates a random pair of private and public binary keys.
func GenKeyPair() ([]byte, []byte, error) {
	return GetAsymProvider().GenKeyPair()
}

// GenHexKeys generates a random pair of private and public hex keys.
func GenHexKeys() (string, string, error) {
	priv, pub, err := GenKeyPair()
	if err != nil {
		return ``, ``, err
	}
	return hex.EncodeToString(priv), PubToHex(pub), nil
}

func Sign(privateKey, data []byte) ([]byte, error) {
	return GetAsymProvider().Sign(privateKey, Hash(data))
}

func Verify(public, data, signature []byte) (bool, error) {
	return GetAsymProvider().Verify(public, Hash(data), signature)
}

// PrivateToPublic returns the public key for the specified private key.
func PrivateToPublic(key []byte) ([]byte, error) {
	return GetAsymProvider().PrivateToPublic(key)
}

func SignString(privateKeyHex, data string) ([]byte, error) {
	privateKey, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("decoding private key from hex: %w", err)
	}
	return Sign(privateKey, []byte(data))
}

func GetHMAC(secret string, message string) ([]byte, error) {
	return GetHashProvider().GetHMAC(secret, message)
}

func Hash(msg []byte) []byte {
	return GetHashProvider().GetHash(msg)
}

func DoubleHash(msg []byte) []byte {
	return GetHashProvider().DoubleHash(msg)
}

func HashHex(input []byte) string {
	return hex.EncodeToString(Hash(input))
}
