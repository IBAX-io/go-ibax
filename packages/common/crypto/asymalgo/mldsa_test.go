package asymalgo

import (
	"bytes"
	"crypto/mldsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

type mldsaSigner interface {
	GenKeyPair() ([]byte, []byte, error)
	Sign(privateKey, hash []byte) ([]byte, error)
	Verify(public, hash, signature []byte) (bool, error)
	PrivateToPublic(key []byte) ([]byte, error)
	PublicKeySize() int
}

// mldsaTestLevels are the ML-DSA cryptoers with what pins each: the parameter set, the size of the
// FIPS 204 expanded private key an imported key could mistakenly have, and the SHA-256 of the
// public key of testSeed, cross-checked with @noble/post-quantum, the implementation Weaver signs
// with.
var mldsaTestLevels = []struct {
	name         string
	signer       mldsaSigner
	params       func() mldsa.Parameters
	expandedSize int
	pubSHA256    string
}{
	{"MLDSA65", &MLDSA65{}, mldsa.MLDSA65, 4032, "d666806e11cee19a7c989f7445f90dd419cf4d2d51db8c0fdb4c0f0a542238c9"},
	{"MLDSA87", &MLDSA87{}, mldsa.MLDSA87, 4896, "91dc389cfaa01470b7f66eee45a4ae9026d154817c754dfe22298b3fa241ffcd"},
}

func testSeed() []byte {
	seed := make([]byte, MLDSASeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	return seed
}

// The seed fixes the key pair (FIPS 204 ML-DSA.KeyGen_internal)
func TestMLDSAPublicKeyFromSeed(t *testing.T) {
	for _, l := range mldsaTestLevels {
		pub, err := l.signer.PrivateToPublic(testSeed())
		if err != nil {
			t.Fatal(err)
		}
		if len(pub) != l.signer.PublicKeySize() {
			t.Fatalf("%s: public key is %d bytes", l.name, len(pub))
		}
		sum := sha256.Sum256(pub)
		if got := hex.EncodeToString(sum[:]); got != l.pubSHA256 {
			t.Fatalf("%s: public key sha256 %s, want %s", l.name, got, l.pubSHA256)
		}
	}
}

// Signing is hedged: two signatures of one hash differ, and both verify
func TestMLDSASignIsHedged(t *testing.T) {
	for _, l := range mldsaTestLevels {
		m := l.signer
		priv, pub, err := m.GenKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256([]byte("block"))
		a, err := m.Sign(priv, hash[:])
		if err != nil {
			t.Fatal(err)
		}
		b, err := m.Sign(priv, hash[:])
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(a, b) {
			t.Fatalf("%s: two signatures of the same hash are identical", l.name)
		}
		for _, sig := range [][]byte{a, b} {
			if ok, err := m.Verify(pub, hash[:], sig); !ok || err != nil {
				t.Fatalf("%s: verify: %v", l.name, err)
			}
		}
	}
}

func TestMLDSARefusesInvalidPrivateKey(t *testing.T) {
	hash := sha256.Sum256([]byte("block"))
	for _, l := range mldsaTestLevels {
		m := l.signer
		keys := map[string][]byte{"nil": nil, "short": make([]byte, 31), "long": make([]byte, 33), "expanded": make([]byte, l.expandedSize)}
		for name, key := range keys {
			if sig, err := m.Sign(key, hash[:]); !errors.Is(err, ErrInvalidPrivateKey) {
				t.Errorf("%s %s: Sign returned %x, %v", l.name, name, sig, err)
			}
			if pub, err := m.PrivateToPublic(key); !errors.Is(err, ErrInvalidPrivateKey) {
				t.Errorf("%s %s: PrivateToPublic returned %x, %v", l.name, name, pub, err)
			}
		}
		if _, err := m.Sign(testSeed(), nil); !errors.Is(err, ErrSigningEmpty) {
			t.Errorf("%s: signed an empty hash: %v", l.name, err)
		}
	}
}

// A signature that does not match is ErrIncorrectSign; malformed input is a different error
func TestMLDSAVerifyRejects(t *testing.T) {
	for _, l := range mldsaTestLevels {
		m := l.signer
		priv := testSeed()
		pub, _ := m.PrivateToPublic(priv)
		hash := sha256.Sum256([]byte("block"))
		sig, err := m.Sign(priv, hash[:])
		if err != nil {
			t.Fatal(err)
		}
		otherPriv := append(testSeed()[1:], 0)
		otherPub, _ := m.PrivateToPublic(otherPriv)
		otherHash := sha256.Sum256([]byte("block2"))
		tampered := append([]byte{}, sig...)
		tampered[100] ^= 1
		// The same key and hash signed without the IBAX context
		sk, err := mldsa.NewPrivateKey(l.params(), priv)
		if err != nil {
			t.Fatal(err)
		}
		noContext, err := sk.Sign(nil, hash[:], nil)
		if err != nil {
			t.Fatal(err)
		}

		for name, c := range map[string]struct{ pub, hash, sig []byte }{
			"other message": {pub, otherHash[:], sig},
			"other key":     {otherPub, hash[:], sig},
			"tampered":      {pub, hash[:], tampered},
			"no context":    {pub, hash[:], noContext},
		} {
			if ok, err := m.Verify(c.pub, c.hash, c.sig); ok || !errors.Is(err, ErrIncorrectSign) {
				t.Errorf("%s %s: Verify returned %v, %v", l.name, name, ok, err)
			}
		}
		for name, c := range map[string]struct{ pub, hash, sig []byte }{
			"no key":       {nil, hash[:], sig},
			"short key":    {pub[:64], hash[:], sig},
			"no hash":      {pub, nil, sig},
			"short sig":    {pub, hash[:], sig[:64]},
			"no signature": {pub, hash[:], nil},
		} {
			if ok, err := m.Verify(c.pub, c.hash, c.sig); ok || err == nil || errors.Is(err, ErrIncorrectSign) {
				t.Errorf("%s %s: Verify returned %v, %v", l.name, name, ok, err)
			}
		}
	}
}

// The levels do not accept each other's keys or signatures
func TestMLDSALevelsAreSeparate(t *testing.T) {
	hash := sha256.Sum256([]byte("block"))
	m65, m87 := &MLDSA65{}, &MLDSA87{}
	pub65, _ := m65.PrivateToPublic(testSeed())
	pub87, _ := m87.PrivateToPublic(testSeed())
	sig65, _ := m65.Sign(testSeed(), hash[:])
	sig87, _ := m87.Sign(testSeed(), hash[:])
	if ok, err := m87.Verify(pub65, hash[:], sig65); ok || err == nil {
		t.Errorf("ML-DSA-87 accepted an ML-DSA-65 key and signature: %v", err)
	}
	if ok, err := m65.Verify(pub87, hash[:], sig87); ok || err == nil {
		t.Errorf("ML-DSA-65 accepted an ML-DSA-87 key and signature: %v", err)
	}
}
