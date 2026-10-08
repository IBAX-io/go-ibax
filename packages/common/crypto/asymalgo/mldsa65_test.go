package asymalgo

import (
	"bytes"
	"crypto/mldsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func testSeed() []byte {
	seed := make([]byte, MLDSA65SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	return seed
}

// The seed fixes the key pair (FIPS 204 ML-DSA.KeyGen_internal); the fingerprint was cross-checked
// with @noble/post-quantum, the implementation Weaver signs with.
func TestMLDSA65PublicKeyFromSeed(t *testing.T) {
	pub, err := (&MLDSA65{}).PrivateToPublic(testSeed())
	if err != nil {
		t.Fatal(err)
	}
	if len(pub) != MLDSA65PublicKeySize {
		t.Fatalf("public key is %d bytes", len(pub))
	}
	sum := sha256.Sum256(pub)
	if got, want := hex.EncodeToString(sum[:]), mldsa65TestPubSHA256; got != want {
		t.Fatalf("public key sha256 %s, want %s", got, want)
	}
}

// Signing is hedged: two signatures of one hash differ, and both verify
func TestMLDSA65SignIsHedged(t *testing.T) {
	m := &MLDSA65{}
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
		t.Fatal("two signatures of the same hash are identical")
	}
	for _, sig := range [][]byte{a, b} {
		if ok, err := m.Verify(pub, hash[:], sig); !ok || err != nil {
			t.Fatalf("verify: %v", err)
		}
	}
}

func TestMLDSA65RefusesInvalidPrivateKey(t *testing.T) {
	m := &MLDSA65{}
	hash := sha256.Sum256([]byte("block"))
	// An ML-DSA-65 private key in the FIPS 204 expanded encoding
	expanded := make([]byte, 4032)
	for name, key := range map[string][]byte{"nil": nil, "short": make([]byte, 31), "long": make([]byte, 33), "expanded": expanded} {
		if sig, err := m.Sign(key, hash[:]); !errors.Is(err, ErrInvalidPrivateKey) {
			t.Errorf("%s: Sign returned %x, %v", name, sig, err)
		}
		if pub, err := m.PrivateToPublic(key); !errors.Is(err, ErrInvalidPrivateKey) {
			t.Errorf("%s: PrivateToPublic returned %x, %v", name, pub, err)
		}
	}
	if _, err := m.Sign(testSeed(), nil); !errors.Is(err, ErrSigningEmpty) {
		t.Errorf("signed an empty hash: %v", err)
	}
}

// A signature that does not match is ErrIncorrectSign; malformed input is a different error
func TestMLDSA65VerifyRejects(t *testing.T) {
	m := &MLDSA65{}
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
	sk, err := mldsa.NewPrivateKey(mldsa.MLDSA65(), priv)
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
			t.Errorf("%s: Verify returned %v, %v", name, ok, err)
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
			t.Errorf("%s: Verify returned %v, %v", name, ok, err)
		}
	}
}

const mldsa65TestPubSHA256 = "d666806e11cee19a7c989f7445f90dd419cf4d2d51db8c0fdb4c0f0a542238c9"
