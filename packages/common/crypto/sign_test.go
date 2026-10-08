package crypto

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
)

var (
	curveAlgos = []AsymAlgo{AsymAlgo_ECC_P256, AsymAlgo_ECC_Secp256k1, AsymAlgo_SM2}
	signAlgos  = []AsymAlgo{AsymAlgo_ECC_P256, AsymAlgo_ECC_Secp256k1, AsymAlgo_SM2, AsymAlgo_MLDSA65}
	hashAlgos  = []HashAlgo{HashAlgo_SHA256, HashAlgo_KECCAK256, HashAlgo_SHA3_256, HashAlgo_SM3}
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
					if len(pub) != asym.PublicKeySize() {
						t.Fatalf("public key is %d bytes, PublicKeySize %d", len(pub), asym.PublicKeySize())
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

// A key that is not a curve scalar (unloaded, zero, or >= n) must be refused, not signed with
func TestSignRefusesInvalidPrivateKey(t *testing.T) {
	digest := NewHashAlgo(HashAlgo_SHA256).GetHash([]byte("block"))
	order := map[AsymAlgo]string{
		AsymAlgo_ECC_P256:      "ffffffff00000000ffffffffffffffffbce6faada7179e84f3b9cac2fc632551",
		AsymAlgo_ECC_Secp256k1: "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141",
		AsymAlgo_SM2:           "fffffffeffffffffffffffffffffffff7203df6b21c6052b53bbf40939d54123",
	}
	for _, a := range curveAlgos {
		n, _ := hex.DecodeString(order[a])
		for name, key := range map[string][]byte{"nil": nil, "zero": make([]byte, 32), "order": n} {
			if sig, err := NewAsymAlgo(a).Sign(key, digest); err == nil {
				t.Errorf("%s: signed with a %s private key: %x", a, name, sig)
			}
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
