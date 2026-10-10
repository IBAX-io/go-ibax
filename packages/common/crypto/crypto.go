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

// GetAsymProvider is the provider of the node algorithm
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

// The node algorithm signs blocks, candidate votes and the requests between nodes, with the keys
// of honor_nodes and the node key files. Accounts sign with the algorithm of their own key
// (AccountKey).

// NodeAlgo is the signature algorithm of the network's node keys
func NodeAlgo() AsymAlgo {
	return asymAlgo
}

// NodeAccountKey is the account public key of a node public key: the node account signs its
// transactions and logins with the node key
func NodeAccountKey(nodePublicKey []byte) AccountKey {
	return AccountKey{Algo: NodeAlgo(), Raw: CutPub(nodePublicKey)}
}

// ParseNodeKeyHex reads a node public key in hex, as honor_nodes and the candidate nodes hold it,
// as the account key of the node
func ParseNodeKeyHex(s string) (AccountKey, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return AccountKey{}, fmt.Errorf("%w: %v", ErrAccountKeyFormat, err)
	}
	return NewAccountKey(NodeAlgo(), CutPub(b))
}

// NodeKeyHex is the node public key in hex, as honor_nodes holds it, of a node account key: a key
// of the network's node algorithm
func (k AccountKey) NodeKeyHex() (string, error) {
	if k.Algo != NodeAlgo() {
		return "", fmt.Errorf("%w: a %s key is no node key of the network (%s)", ErrAccountKeyFormat, k.Algo, NodeAlgo())
	}
	return PubToHex(k.Raw), nil
}

// GenNodeKeyPair generates a random pair of private and public node keys
func GenNodeKeyPair() ([]byte, []byte, error) {
	return GetAsymProvider().GenKeyPair()
}

func NodeSign(privateKey, data []byte) ([]byte, error) {
	return GetAsymProvider().Sign(privateKey, Hash(data))
}

func NodeVerify(public, data, signature []byte) (bool, error) {
	return GetAsymProvider().Verify(public, Hash(data), signature)
}

// NodePublicKeySize is the length of the node public keys (without the 04 prefix of the curves)
func NodePublicKeySize() int {
	return GetAsymProvider().PublicKeySize()
}

// NodeSignatureSize is the maximum length of the node signatures
func NodeSignatureSize() int {
	return GetAsymProvider().SignatureSize()
}

// NodePrivateToPublic returns the node public key of a node private key
func NodePrivateToPublic(key []byte) ([]byte, error) {
	return GetAsymProvider().PrivateToPublic(key)
}

func NodeSignString(privateKeyHex, data string) ([]byte, error) {
	privateKey, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("decoding private key from hex: %w", err)
	}
	return NodeSign(privateKey, []byte(data))
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
