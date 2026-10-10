/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package crypto

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
)

// AccountKeyMarker opens every account public key. It is neither an SEC1 point prefix (02, 03,
// 04) nor the DER sequence tag (30), so an account key is never taken for a bare curve key.
const AccountKeyMarker = 0xAC

// ErrAccountKeyFormat is an account public key that is not the marker, a known algorithm and a
// valid public key of that algorithm
var ErrAccountKeyFormat = errors.New("invalid account public key")

// AccountKey is an account public key and the algorithm it signs with. Each account carries its
// own algorithm, so accounts of different algorithms share one chain; the node keys keep the
// algorithm of the network (NodeSign, NodeVerify). Its encoding, kept in 1_keys.pub, sent in the
// transaction header and at login, is
//
//	0xAC | algorithm (AsymAlgo, one byte) | public key as the algorithm's provider returns it
//
// The address is that of the bare public key.
type AccountKey struct {
	Algo AsymAlgo
	Raw  []byte
}

// AccountAlgoImplemented reports whether accounts can hold keys of the algorithm: it is in the
// enum and the node implements it
func AccountAlgoImplemented(a AsymAlgo) bool {
	_, known := AsymAlgo_name[int32(a)]
	return known && a != AsymAlgo_ECC_P512
}

// NewAccountKey checks that raw is a public key of algo
func NewAccountKey(algo AsymAlgo, raw []byte) (AccountKey, error) {
	if !AccountAlgoImplemented(algo) {
		return AccountKey{}, fmt.Errorf("%w: algorithm %d is not implemented", ErrAccountKeyFormat, algo)
	}
	if err := NewAsymAlgo(algo).CheckPublicKey(raw); err != nil {
		return AccountKey{}, fmt.Errorf("%w: not a %s public key", ErrAccountKeyFormat, algo)
	}
	return AccountKey{Algo: algo, Raw: bytes.Clone(raw)}, nil
}

// ParseAccountKey reads an encoded account public key
func ParseAccountKey(b []byte) (AccountKey, error) {
	if len(b) < 2 || b[0] != AccountKeyMarker {
		return AccountKey{}, fmt.Errorf("%w: no 0x%X marker", ErrAccountKeyFormat, AccountKeyMarker)
	}
	return NewAccountKey(AsymAlgo(b[1]), b[2:])
}

// ParseAccountKeyHex reads an account public key in hex
func ParseAccountKeyHex(s string) (AccountKey, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return AccountKey{}, fmt.Errorf("%w: %v", ErrAccountKeyFormat, err)
	}
	return ParseAccountKey(b)
}

// Bytes is the encoding of the key
func (k AccountKey) Bytes() []byte {
	return append([]byte{AccountKeyMarker, byte(k.Algo)}, k.Raw...)
}

// Hex is the encoding of the key in hex
func (k AccountKey) Hex() string {
	return hex.EncodeToString(k.Bytes())
}

// Address is the account of the key
func (k AccountKey) Address() int64 {
	return Address(k.Raw)
}

// Verify checks a signature of data, hashed with the network hash, made with the key's algorithm
func (k AccountKey) Verify(data, signature []byte) (bool, error) {
	return NewAsymAlgo(k.Algo).Verify(k.Raw, Hash(data), signature)
}

// GenAccountKey generates a private key and its account public key
func GenAccountKey(algo AsymAlgo) ([]byte, AccountKey, error) {
	if !AccountAlgoImplemented(algo) {
		return nil, AccountKey{}, fmt.Errorf("%w: algorithm %d is not implemented", ErrAccountKeyFormat, algo)
	}
	priv, pub, err := NewAsymAlgo(algo).GenKeyPair()
	if err != nil {
		return nil, AccountKey{}, err
	}
	return priv, AccountKey{Algo: algo, Raw: pub}, nil
}

// AccountKeyOf is the account public key of a private key of algo
func AccountKeyOf(algo AsymAlgo, privateKey []byte) (AccountKey, error) {
	if !AccountAlgoImplemented(algo) {
		return AccountKey{}, fmt.Errorf("%w: algorithm %d is not implemented", ErrAccountKeyFormat, algo)
	}
	pub, err := NewAsymAlgo(algo).PrivateToPublic(privateKey)
	if err != nil {
		return AccountKey{}, err
	}
	return NewAccountKey(algo, pub)
}

// AccountSign signs data, hashed with the network hash, with a private key of algo
func AccountSign(algo AsymAlgo, privateKey, data []byte) ([]byte, error) {
	if !AccountAlgoImplemented(algo) {
		return nil, fmt.Errorf("%w: algorithm %d is not implemented", ErrAccountKeyFormat, algo)
	}
	return NewAsymAlgo(algo).Sign(privateKey, Hash(data))
}
