package asymalgo

import (
	"crypto/mldsa"
	"fmt"
)

const (
	// MLDSA65Context separates IBAX signatures from ML-DSA-65 signatures made for any other purpose
	// (FIPS 204 context string). Changing it invalidates every key's existing signatures.
	MLDSA65Context = "IBAX-MLDSA-65-v1"
	// MLDSA65SeedSize is the private key: the FIPS 204 seed the key pair is expanded from
	MLDSA65SeedSize = mldsa.PrivateKeySize
	// MLDSA65PublicKeySize is the size of an encoded public key
	MLDSA65PublicKeySize = mldsa.MLDSA65PublicKeySize
	// MLDSA65SignatureSize is the size of a signature
	MLDSA65SignatureSize = mldsa.MLDSA65SignatureSize
)

var mldsa65Options = &mldsa.Options{Context: MLDSA65Context}

// MLDSA65 is ML-DSA-65 (FIPS 204). The private key is the 32-byte seed rather than the 4032-byte
// expanded key, so it is stored, encrypted and imported like the private keys of the curves.
// Signatures are pure ML-DSA over the hash the caller passes, hedged with fresh randomness.
type MLDSA65 struct{}

func (m *MLDSA65) GenKeyPair() ([]byte, []byte, error) {
	sk, err := mldsa.GenerateKey(mldsa.MLDSA65())
	if err != nil {
		return nil, nil, err
	}
	return sk.Bytes(), sk.PublicKey().Bytes(), nil
}

func (m *MLDSA65) Sign(privateKey, hash []byte) ([]byte, error) {
	if len(hash) == 0 {
		return nil, ErrSigningEmpty
	}
	sk, err := mldsa65Key(privateKey)
	if err != nil {
		return nil, err
	}
	// The reader is ignored: the standard library always draws the hedging randomness itself
	return sk.Sign(nil, hash, mldsa65Options)
}

func (m *MLDSA65) Verify(public, hash, signature []byte) (bool, error) {
	if len(public) == 0 {
		return false, ErrCheckingSignEmpty
	}
	if len(hash) == 0 {
		return false, fmt.Errorf("invalid parameters len(data) == 0")
	}
	if len(public) != MLDSA65PublicKeySize {
		return false, fmt.Errorf("invalid parameters len(public) = %d", len(public))
	}
	if len(signature) != MLDSA65SignatureSize {
		return false, fmt.Errorf("invalid parameters len(signature) = %d", len(signature))
	}
	pk, err := mldsa.NewPublicKey(mldsa.MLDSA65(), public)
	if err != nil {
		return false, err
	}
	if err := mldsa.Verify(pk, hash, signature, mldsa65Options); err != nil {
		return false, ErrIncorrectSign
	}
	return true, nil
}

func (m *MLDSA65) PrivateToPublic(key []byte) ([]byte, error) {
	sk, err := mldsa65Key(key)
	if err != nil {
		return nil, err
	}
	return sk.PublicKey().Bytes(), nil
}

func (m *MLDSA65) PublicKeySize() int {
	return MLDSA65PublicKeySize
}

// mldsa65Key expands a seed into its key pair. Any 32 bytes are a valid seed, so only the length
// can be wrong; an expanded 4032-byte key is refused rather than taken for something else.
func mldsa65Key(seed []byte) (*mldsa.PrivateKey, error) {
	if len(seed) != MLDSA65SeedSize {
		return nil, ErrInvalidPrivateKey
	}
	return mldsa.NewPrivateKey(mldsa.MLDSA65(), seed)
}
