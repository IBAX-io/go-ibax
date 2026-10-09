/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package crypto

import (
	"crypto/sha256"
	"crypto/subtle"
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
	case AsymAlgo_MLDSA65:
		return &asymalgo.MLDSA65{}
	case AsymAlgo_MLDSA87:
		return &asymalgo.MLDSA87{}
	}
	panic(fmt.Errorf("curve algo [%v] is not supported yet", a))
}

func InitAsymAlgo(s string) {
	v, ok := AsymAlgo_value[s]
	if !ok {
		log.Fatal(fmt.Errorf("curve algo [%v] is not supported yet, Run 'go-ibax config --help' for details", s))
	}
	if err := CheckAsymAlgo(AsymAlgo(v)); err != nil {
		log.Fatal(err)
	}
	asymAlgo = AsymAlgo(v)
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
	case HashAlgo_SHA384:
		return &hashalgo.SHA384{}
	case HashAlgo_SHA512:
		return &hashalgo.SHA512{}
	}
	panic(fmt.Errorf("hash algo [%v] is not supported yet", a))
}

func InitHashAlgo(s string) {
	v, ok := HashAlgo_value[s]
	if !ok {
		log.Fatal(fmt.Errorf("hash algo [%v] is not supported yet, Run 'go-ibax config --help' for details", s))
	}
	if err := CheckHashAlgo(HashAlgo(v)); err != nil {
		log.Fatal(err)
	}
	hashAlgo = HashAlgo(v)
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

// PublicKeySize is the length of the network's public keys (without the 04 prefix of the curves)
func PublicKeySize() int {
	return GetAsymProvider().PublicKeySize()
}

// SignatureSize is the maximum length of the network's signatures
func SignatureSize() int {
	return GetAsymProvider().SignatureSize()
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
	if err := CheckHMACKey(len(secret)); err != nil {
		return nil, err
	}
	return GetHashProvider().GetHMAC(secret, message)
}

func Hash(msg []byte) []byte {
	return GetHashProvider().GetHash(msg)
}

func DoubleHash(msg []byte) []byte {
	return GetHashProvider().DoubleHash(msg)
}

// HashSize is the length in bytes of the network's hashes: block, transaction and data hashes
func HashSize() int {
	return GetHashProvider().Size()
}

func HashHex(input []byte) string {
	return hex.EncodeToString(Hash(input))
}

// MatchesDataHash reports whether hexHash, taken from a /data link, is the hash of data. Binaries
// are linked by the network hash (the Hash contract function); dbfind links blob and long text
// values by their SHA-256, which the database computes.
func MatchesDataHash(data []byte, hexHash string) bool {
	want, err := hex.DecodeString(hexHash)
	if err != nil {
		return false
	}
	if len(want) == sha256.Size {
		if sum := sha256.Sum256(data); subtle.ConstantTimeCompare(sum[:], want) == 1 {
			return true
		}
	}
	return len(want) == HashSize() && subtle.ConstantTimeCompare(Hash(data), want) == 1
}
