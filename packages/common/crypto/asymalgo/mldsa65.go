package asymalgo

import (
	crand "crypto/rand"
	"fmt"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

const (
	// MLDSA65Context separates IBAX signatures from ML-DSA-65 signatures made for any other purpose
	// (FIPS 204 context string). Changing it invalidates every key's existing signatures.
	MLDSA65Context = "IBAX-MLDSA-65-v1"
	// MLDSA65SeedSize is the private key: the FIPS 204 seed the key pair is expanded from
	MLDSA65SeedSize = mldsa65.SeedSize
	// MLDSA65PublicKeySize is the size of an encoded public key
	MLDSA65PublicKeySize = mldsa65.PublicKeySize
	// MLDSA65SignatureSize is the size of a signature
	MLDSA65SignatureSize = mldsa65.SignatureSize
)

// MLDSA65 is ML-DSA-65 (FIPS 204). The private key is the 32-byte seed rather than the 4032-byte
// expanded key, so it is stored, encrypted and imported like the private keys of the curves.
// Signatures are pure ML-DSA over the hash the caller passes, hedged with fresh randomness.
type MLDSA65 struct{}

func (m *MLDSA65) GenKeyPair() ([]byte, []byte, error) {
	seed := make([]byte, MLDSA65SeedSize)
	if _, err := crand.Read(seed); err != nil {
		return nil, nil, err
	}
	pub, err := m.PrivateToPublic(seed)
	if err != nil {
		return nil, nil, err
	}
	return seed, pub, nil
}

func (m *MLDSA65) Sign(privateKey, hash []byte) ([]byte, error) {
	if len(hash) == 0 {
		return nil, ErrSigningEmpty
	}
	_, sk, err := mldsa65Key(privateKey)
	if err != nil {
		return nil, err
	}
	sig := make([]byte, MLDSA65SignatureSize)
	if err := mldsa65.SignTo(sk, hash, []byte(MLDSA65Context), true, sig); err != nil {
		return nil, err
	}
	return sig, nil
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
	var pk mldsa65.PublicKey
	if err := pk.UnmarshalBinary(public); err != nil {
		return false, err
	}
	if !mldsa65.Verify(&pk, hash, []byte(MLDSA65Context), signature) {
		return false, ErrIncorrectSign
	}
	return true, nil
}

func (m *MLDSA65) PrivateToPublic(key []byte) ([]byte, error) {
	pk, _, err := mldsa65Key(key)
	if err != nil {
		return nil, err
	}
	return pk.Bytes(), nil
}

func (m *MLDSA65) PublicKeySize() int {
	return MLDSA65PublicKeySize
}

// mldsa65Key expands a seed into its key pair. Any 32 bytes are a valid seed, so only the length
// can be wrong; an expanded 4032-byte key is refused rather than taken for something else.
func mldsa65Key(seed []byte) (*mldsa65.PublicKey, *mldsa65.PrivateKey, error) {
	if len(seed) != MLDSA65SeedSize {
		return nil, nil, ErrInvalidPrivateKey
	}
	var s [MLDSA65SeedSize]byte
	copy(s[:], seed)
	pk, sk := mldsa65.NewKeyFromSeed(&s)
	return pk, sk, nil
}
