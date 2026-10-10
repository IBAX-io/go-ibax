package asymalgo

import (
	"crypto"
	crand "crypto/rand"
	"crypto/rsa"
	_ "crypto/sha256"
	_ "crypto/sha3"
	_ "crypto/sha512"
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"
)

// RSAExponent is the public exponent of every RSA account key: PIV keys have no other
// (SP 800-78-4), so the public key is the modulus alone
const RSAExponent = 65537

// ErrRSAHash is a network hash RSASSA-PSS is not defined with
var ErrRSAHash = errors.New("RSASSA-PSS is not defined with the network hash")

// RSA is RSASSA-PSS (FIPS 186-5 5.4) with a modulus of Bits bits, a PIV key and an account
// algorithm only. The PSS hash and the MGF1 hash are the network hash, Hash, and the salt is as
// long as the hash; Hash is zero when the network hash is not SHA-2 or SHA3-256, and RSA neither
// signs nor verifies. The public key is the modulus, big-endian, Bits/8 bytes with the top bit
// set; the signature is Bits/8 bytes; the private key is PKCS #1 DER.
type RSA struct {
	Bits int
	Hash crypto.Hash
}

func (e *RSA) size() int {
	return e.Bits / 8
}

func (e *RSA) options() (*rsa.PSSOptions, error) {
	if e.Hash == 0 || !e.Hash.Available() {
		return nil, ErrRSAHash
	}
	return &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: e.Hash}, nil
}

func (e *RSA) GenKeyPair() ([]byte, []byte, error) {
	priv, err := rsa.GenerateKey(crand.Reader, e.Bits)
	if err != nil {
		return nil, nil, err
	}
	return x509.MarshalPKCS1PrivateKey(priv), priv.N.FillBytes(make([]byte, e.size())), nil
}

func (e *RSA) private(key []byte) (*rsa.PrivateKey, error) {
	priv, err := x509.ParsePKCS1PrivateKey(key)
	if err != nil || priv.N.BitLen() != e.Bits || priv.E != RSAExponent {
		return nil, ErrInvalidPrivateKey
	}
	return priv, nil
}

func (e *RSA) public(public []byte) (*rsa.PublicKey, error) {
	if err := e.CheckPublicKey(public); err != nil {
		return nil, err
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(public), E: RSAExponent}, nil
}

func (e *RSA) Sign(privateKey, hash []byte) ([]byte, error) {
	if len(hash) == 0 {
		return nil, ErrSigningEmpty
	}
	opts, err := e.options()
	if err != nil {
		return nil, err
	}
	if len(hash) != e.Hash.Size() {
		return nil, fmt.Errorf("invalid parameters len(hash) = %d", len(hash))
	}
	priv, err := e.private(privateKey)
	if err != nil {
		return nil, err
	}
	return rsa.SignPSS(crand.Reader, priv, e.Hash, hash, opts)
}

func (e *RSA) Verify(public, hash, signature []byte) (bool, error) {
	if len(public) == 0 {
		return false, ErrCheckingSignEmpty
	}
	opts, err := e.options()
	if err != nil {
		return false, err
	}
	if len(hash) != e.Hash.Size() {
		return false, fmt.Errorf("invalid parameters len(hash) = %d", len(hash))
	}
	if len(signature) != e.size() {
		return false, fmt.Errorf("invalid parameters len(signature) = %d", len(signature))
	}
	pub, err := e.public(public)
	if err != nil {
		return false, err
	}
	if rsa.VerifyPSS(pub, e.Hash, hash, signature, opts) != nil {
		return false, ErrIncorrectSign
	}
	return true, nil
}

func (e *RSA) PrivateToPublic(key []byte) ([]byte, error) {
	priv, err := e.private(key)
	if err != nil {
		return nil, err
	}
	return priv.N.FillBytes(make([]byte, e.size())), nil
}

// CheckPublicKey refuses a modulus that is not exactly Bits bits or is even
func (e *RSA) CheckPublicKey(public []byte) error {
	if len(public) != e.size() || public[0]&0x80 == 0 || public[len(public)-1]&1 == 0 {
		return ErrInvalidPublicKey
	}
	return nil
}

func (e *RSA) PublicKeySize() int {
	return e.size()
}

func (e *RSA) SignatureSize() int {
	return e.size()
}
