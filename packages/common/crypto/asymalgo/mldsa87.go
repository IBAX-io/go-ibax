package asymalgo

import "crypto/mldsa"

const (
	// MLDSA87Context separates IBAX signatures from ML-DSA-87 signatures made for any other purpose
	// (FIPS 204 context string). Changing it invalidates every key's existing signatures.
	MLDSA87Context = "IBAX-MLDSA-87-v1"
	// MLDSA87PublicKeySize is the size of an encoded public key
	MLDSA87PublicKeySize = mldsa.MLDSA87PublicKeySize
	// MLDSA87SignatureSize is the size of a signature
	MLDSA87SignatureSize = mldsa.MLDSA87SignatureSize
)

var mldsa87 = &mldsaLevel{
	params:        mldsa.MLDSA87,
	options:       &mldsa.Options{Context: MLDSA87Context},
	publicKeySize: MLDSA87PublicKeySize,
	signatureSize: MLDSA87SignatureSize,
}

// MLDSA87 is ML-DSA-87 (FIPS 204, security category 5, the level CNSA 2.0 requires)
type MLDSA87 struct{}

func (m *MLDSA87) GenKeyPair() ([]byte, []byte, error) { return mldsa87.genKeyPair() }
func (m *MLDSA87) Sign(privateKey, hash []byte) ([]byte, error) {
	return mldsa87.sign(privateKey, hash)
}
func (m *MLDSA87) Verify(public, hash, signature []byte) (bool, error) {
	return mldsa87.verify(public, hash, signature)
}
func (m *MLDSA87) PrivateToPublic(key []byte) ([]byte, error) { return mldsa87.privateToPublic(key) }
func (m *MLDSA87) PublicKeySize() int                         { return MLDSA87PublicKeySize }
func (m *MLDSA87) SignatureSize() int                         { return MLDSA87SignatureSize }
