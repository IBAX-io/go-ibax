package asymalgo

import "crypto/mldsa"

const (
	// MLDSA65Context separates IBAX signatures from ML-DSA-65 signatures made for any other purpose
	// (FIPS 204 context string). Changing it invalidates every key's existing signatures.
	MLDSA65Context = "IBAX-MLDSA-65-v1"
	// MLDSA65PublicKeySize is the size of an encoded public key
	MLDSA65PublicKeySize = mldsa.MLDSA65PublicKeySize
	// MLDSA65SignatureSize is the size of a signature
	MLDSA65SignatureSize = mldsa.MLDSA65SignatureSize
)

var mldsa65 = &mldsaLevel{
	params:        mldsa.MLDSA65,
	options:       &mldsa.Options{Context: MLDSA65Context},
	publicKeySize: MLDSA65PublicKeySize,
	signatureSize: MLDSA65SignatureSize,
}

// MLDSA65 is ML-DSA-65 (FIPS 204, security category 3)
type MLDSA65 struct{}

func (m *MLDSA65) GenKeyPair() ([]byte, []byte, error) { return mldsa65.genKeyPair() }
func (m *MLDSA65) Sign(privateKey, hash []byte) ([]byte, error) {
	return mldsa65.sign(privateKey, hash)
}
func (m *MLDSA65) Verify(public, hash, signature []byte) (bool, error) {
	return mldsa65.verify(public, hash, signature)
}
func (m *MLDSA65) PrivateToPublic(key []byte) ([]byte, error) { return mldsa65.privateToPublic(key) }
func (m *MLDSA65) PublicKeySize() int                         { return MLDSA65PublicKeySize }
func (m *MLDSA65) SignatureSize() int                         { return MLDSA65SignatureSize }
