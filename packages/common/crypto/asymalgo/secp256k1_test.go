package asymalgo

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha512"
	"math/big"
	"testing"

	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacySecpVerify is the verification Secp256k1 had with ecdsa.Verify of the standard library
func legacySecpVerify(public, hash, signature []byte) bool {
	pub := &ecdsa.PublicKey{Curve: secp.S256(), X: new(big.Int).SetBytes(public[:32]), Y: new(big.Int).SetBytes(public[32:])}
	return ecdsa.Verify(pub, hash, new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:]))
}

// TestSecp256k1SameVerdicts checks that Verify accepts exactly the signatures the standard
// library accepted: valid ones, of 32 and 64-byte hashes, high s, altered ones, r or s out of range
func TestSecp256k1SameVerdicts(t *testing.T) {
	s := &Secp256k1{}
	n := secp.S256().Params().N
	for i := 0; i < 50; i++ {
		priv, pub, err := s.GenKeyPair()
		require.NoError(t, err)
		msg := make([]byte, 40)
		rand.Read(msg)
		h64 := sha512.Sum512(msg)
		for _, hash := range [][]byte{h64[:32], h64[:]} {
			sig, err := s.Sign(priv, hash)
			require.NoError(t, err)
			highS := append(append([]byte{}, sig[:32]...), FillLeft(new(big.Int).Sub(n, new(big.Int).SetBytes(sig[32:])).Bytes())...)
			altered := append([]byte{}, sig...)
			altered[i%64] ^= 1
			zeroR := append(make([]byte, 32), sig[32:]...)
			bigS := append(append([]byte{}, sig[:32]...), FillLeft(new(big.Int).Add(n, new(big.Int).SetBytes(sig[32:])).Bytes())...)
			for _, c := range [][]byte{sig, highS, altered, zeroR, bigS} {
				if len(c) != 64 {
					continue
				}
				ok, _ := s.Verify(pub, hash, c)
				assert.Equal(t, legacySecpVerify(pub, hash, c), ok)
			}
			ok, err := s.Verify(pub, hash, sig)
			assert.True(t, ok)
			assert.NoError(t, err)
		}
	}
}

func BenchmarkSecp256k1Verify(b *testing.B) {
	s := &Secp256k1{}
	priv, pub, _ := s.GenKeyPair()
	hash := make([]byte, 32)
	sig, _ := s.Sign(priv, hash)
	b.Run("legacy", func(b *testing.B) {
		for b.Loop() {
			legacySecpVerify(pub, hash, sig)
		}
	})
	b.Run("decred", func(b *testing.B) {
		for b.Loop() {
			s.Verify(pub, hash, sig)
		}
	})
}
