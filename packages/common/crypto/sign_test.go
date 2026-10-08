package crypto

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"
)

var (
	signAlgos = []AsymAlgo{AsymAlgo_ECC_P256, AsymAlgo_ECC_Secp256k1, AsymAlgo_SM2}
	hashAlgos = []HashAlgo{HashAlgo_SHA256, HashAlgo_KECCAK256, HashAlgo_SHA3_256, HashAlgo_SM3}
)

const signRounds = 1000

// Every signature algorithm a chain can be configured with must sign and verify under every hash;
// nodes sign blocks and system transactions with it.
func TestSignVerifyEverySuite(t *testing.T) {
	for _, a := range signAlgos {
		for _, h := range hashAlgos {
			a, h := a, h
			t.Run(fmt.Sprintf("%s/%s", a, h), func(t *testing.T) {
				t.Parallel()
				asym, hasher := NewAsymAlgo(a), NewHashAlgo(h)
				msg := make([]byte, 64)
				for i := 0; i < signRounds; i++ {
					priv, pub, err := asym.GenKeyPair()
					if err != nil {
						t.Fatal(err)
					}
					derived, err := asym.PrivateToPublic(priv)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(derived, pub) {
						t.Fatalf("PrivateToPublic %x != generated %x", derived, pub)
					}
					if _, err := rand.Read(msg); err != nil {
						t.Fatal(err)
					}
					digest := hasher.GetHash(msg)
					sig := signNoPanic(t, asym, priv, digest)
					if ok, err := asym.Verify(pub, digest, sig); !ok || err != nil {
						t.Fatalf("verify own signature: %v", err)
					}
					other := hasher.GetHash(append(msg, 0))
					if ok, _ := asym.Verify(pub, other, sig); ok {
						t.Fatal("signature verified for another message")
					}
				}
			})
		}
	}
}

func signNoPanic(t *testing.T, asym AsymProvider, priv, digest []byte) (sig []byte) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Sign panicked: %v", r)
		}
	}()
	sig, err := asym.Sign(priv, digest)
	if err != nil {
		t.Fatal(err)
	}
	return sig
}
