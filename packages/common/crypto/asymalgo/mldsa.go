package asymalgo

import (
	"crypto/mldsa"
	"fmt"
)

// MLDSASeedSize is the private key of every ML-DSA level: the FIPS 204 seed the key pair is
// expanded from
const MLDSASeedSize = mldsa.PrivateKeySize

// mldsaLevel is one ML-DSA parameter set (FIPS 204) with its IBAX context string. The private key
// is the 32-byte seed rather than the expanded key, so it is stored, encrypted and imported like
// the private keys of the curves. Signatures are pure ML-DSA over the hash the caller passes,
// hedged with fresh randomness.
type mldsaLevel struct {
	params        func() mldsa.Parameters
	options       *mldsa.Options
	publicKeySize int
	signatureSize int
}

func (l *mldsaLevel) genKeyPair() ([]byte, []byte, error) {
	sk, err := mldsa.GenerateKey(l.params())
	if err != nil {
		return nil, nil, err
	}
	return sk.Bytes(), sk.PublicKey().Bytes(), nil
}

func (l *mldsaLevel) sign(privateKey, hash []byte) ([]byte, error) {
	if len(hash) == 0 {
		return nil, ErrSigningEmpty
	}
	sk, err := l.key(privateKey)
	if err != nil {
		return nil, err
	}
	// The reader is ignored: the standard library always draws the hedging randomness itself
	return sk.Sign(nil, hash, l.options)
}

func (l *mldsaLevel) verify(public, hash, signature []byte) (bool, error) {
	if len(public) == 0 {
		return false, ErrCheckingSignEmpty
	}
	if len(hash) == 0 {
		return false, fmt.Errorf("invalid parameters len(data) == 0")
	}
	if len(public) != l.publicKeySize {
		return false, fmt.Errorf("invalid parameters len(public) = %d", len(public))
	}
	if len(signature) != l.signatureSize {
		return false, fmt.Errorf("invalid parameters len(signature) = %d", len(signature))
	}
	pk, err := mldsa.NewPublicKey(l.params(), public)
	if err != nil {
		return false, err
	}
	if err := mldsa.Verify(pk, hash, signature, l.options); err != nil {
		return false, ErrIncorrectSign
	}
	return true, nil
}

func (l *mldsaLevel) privateToPublic(key []byte) ([]byte, error) {
	sk, err := l.key(key)
	if err != nil {
		return nil, err
	}
	return sk.PublicKey().Bytes(), nil
}

// key expands a seed into its key pair. Any 32 bytes are a valid seed, so only the length can be
// wrong; an expanded key is refused rather than taken for something else.
func (l *mldsaLevel) key(seed []byte) (*mldsa.PrivateKey, error) {
	if len(seed) != MLDSASeedSize {
		return nil, ErrInvalidPrivateKey
	}
	return mldsa.NewPrivateKey(l.params(), seed)
}

// MLDSAAvailable reports whether the Go Cryptographic Module the node is built with implements
// ML-DSA. Module v1.0.0 (GOFIPS140=v1.0.0) does not: crypto/mldsa then fails on every key.
func MLDSAAvailable() bool {
	_, err := mldsa.NewPrivateKey(mldsa.MLDSA65(), make([]byte, MLDSASeedSize))
	return err == nil
}
