package asymalgo

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"fmt"
	"math/big"
)

// p384Size is the length of a P-384 scalar and of each coordinate
const p384Size = 48

// P384 is ECDSA on P-384, a PIV key and an account algorithm only. Its public key is x || y and
// its signature r || s, each 48 bytes; its private key is the 48-byte scalar.
type P384 struct{}

func (e *P384) GenKeyPair() ([]byte, []byte, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P384(), crand.Reader)
	if err != nil {
		return nil, nil, err
	}
	d, err := priv.Bytes()
	if err != nil {
		return nil, nil, err
	}
	pub, err := priv.PublicKey.Bytes()
	if err != nil {
		return nil, nil, err
	}
	return d, pub[1:], nil
}

func (e *P384) private(key []byte) (*ecdsa.PrivateKey, error) {
	priv, err := ecdsa.ParseRawPrivateKey(elliptic.P384(), key)
	if err != nil {
		return nil, ErrInvalidPrivateKey
	}
	return priv, nil
}

func (e *P384) public(public []byte) (*ecdsa.PublicKey, error) {
	if len(public) != 2*p384Size {
		return nil, ErrInvalidPublicKey
	}
	// The point is refused off the curve, at infinity, or with coordinates not below p
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P384(), append([]byte{4}, public...))
	if err != nil {
		return nil, ErrInvalidPublicKey
	}
	return pub, nil
}

func (e *P384) Sign(privateKey, hash []byte) ([]byte, error) {
	if len(hash) == 0 {
		return nil, ErrSigningEmpty
	}
	priv, err := e.private(privateKey)
	if err != nil {
		return nil, err
	}
	r, s, err := ecdsa.Sign(crand.Reader, priv, hash)
	if err != nil {
		return nil, err
	}
	return append(r.FillBytes(make([]byte, p384Size)), s.FillBytes(make([]byte, p384Size))...), nil
}

func (e *P384) Verify(public, hash, signature []byte) (bool, error) {
	if len(public) == 0 {
		return false, ErrCheckingSignEmpty
	}
	if len(hash) == 0 {
		return false, fmt.Errorf("invalid parameters len(data) == 0")
	}
	if len(signature) != 2*p384Size {
		return false, fmt.Errorf("invalid parameters len(signature) = %d", len(signature))
	}
	pub, err := e.public(public)
	if err != nil {
		return false, err
	}
	r := new(big.Int).SetBytes(signature[:p384Size])
	s := new(big.Int).SetBytes(signature[p384Size:])
	if !ecdsa.Verify(pub, hash, r, s) {
		return false, ErrIncorrectSign
	}
	return true, nil
}

func (e *P384) PrivateToPublic(key []byte) ([]byte, error) {
	priv, err := e.private(key)
	if err != nil {
		return nil, err
	}
	pub, err := priv.PublicKey.Bytes()
	if err != nil {
		return nil, err
	}
	return pub[1:], nil
}

func (e *P384) CheckPublicKey(public []byte) error {
	_, err := e.public(public)
	return err
}

func (e *P384) PublicKeySize() int {
	return 2 * p384Size
}

func (e *P384) SignatureSize() int {
	return 2 * p384Size
}
