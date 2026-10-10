package crypto

import (
	"bytes"
	stdcrypto "crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto/asymalgo"
)

// withHash runs f on a network of hash h
func withHash(h HashAlgo, f func()) {
	defer func(prev HashAlgo) { hashAlgo = prev }(hashAlgo)
	hashAlgo = h
	f()
}

type pivKey struct {
	priv, pub []byte
}

// pivKeys generates one key of each PIV algorithm: RSA keys take long to generate
func pivKeys(t *testing.T) map[AsymAlgo]pivKey {
	t.Helper()
	keys := map[AsymAlgo]pivKey{}
	for _, a := range pivAlgos {
		priv, pub, err := NewAsymAlgo(a).GenKeyPair()
		if err != nil {
			t.Fatalf("%s: %v", a, err)
		}
		keys[a] = pivKey{priv, pub}
	}
	return keys
}

// The PIV algorithms sign and verify under every hash RSASSA-PSS is defined with; RSA neither
// signs nor verifies under KECCAK256 and SM3
func TestPIVAlgosEveryHash(t *testing.T) {
	keys := pivKeys(t)
	for _, a := range pivAlgos {
		for _, h := range hashAlgos {
			t.Run(fmt.Sprintf("%s/%s", a, h), func(t *testing.T) {
				withHash(h, func() {
					asym, key := NewAsymAlgo(a), keys[a]
					if derived, err := asym.PrivateToPublic(key.priv); err != nil || !bytes.Equal(derived, key.pub) {
						t.Fatalf("PrivateToPublic: %v", err)
					}
					if len(key.pub) != asym.PublicKeySize() {
						t.Fatalf("public key is %d bytes, PublicKeySize %d", len(key.pub), asym.PublicKeySize())
					}
					digest := Hash([]byte("transfer " + a.String()))
					sig, err := asym.Sign(key.priv, digest)
					if IsRSAAlgo(a) && PSSHash(h) == 0 {
						if !errors.Is(err, asymalgo.ErrRSAHash) {
							t.Fatalf("signed under %s: %v", h, err)
						}
						if ok, err := asym.Verify(key.pub, digest, make([]byte, asym.SignatureSize())); ok || !errors.Is(err, asymalgo.ErrRSAHash) {
							t.Fatalf("verified under %s: %v", h, err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if len(sig) != asym.SignatureSize() {
						t.Fatalf("signature is %d bytes, SignatureSize %d", len(sig), asym.SignatureSize())
					}
					if ok, err := asym.Verify(key.pub, digest, sig); !ok || err != nil {
						t.Fatalf("verify own signature: %v", err)
					}
					if ok, _ := asym.Verify(key.pub, Hash([]byte("transfer!")), sig); ok {
						t.Fatal("verified for another message")
					}
					tampered := bytes.Clone(sig)
					tampered[len(tampered)-1] ^= 1
					if ok, _ := asym.Verify(key.pub, digest, tampered); ok {
						t.Fatal("verified a tampered signature")
					}
				})
			})
		}
	}
}

// Public keys that are not keys of the algorithm are refused, and so are keys of one PIV
// algorithm labelled as another
func TestPIVPublicKeysRefused(t *testing.T) {
	keys := pivKeys(t)
	p384, rsa2048, rsa3072 := keys[AsymAlgo_ECC_P384].pub, keys[AsymAlgo_RSA2048].pub, keys[AsymAlgo_RSA3072].pub
	offCurve := bytes.Clone(p384)
	offCurve[95] ^= 1
	even := bytes.Clone(rsa2048)
	even[255] &^= 1
	short := bytes.Clone(rsa2048)
	short[0] &^= 0x80
	tagged := func(a AsymAlgo, raw []byte) []byte { return append([]byte{AccountKeyMarker, byte(a)}, raw...) }
	cases := map[string][]byte{
		"P-384 point off curve":   tagged(AsymAlgo_ECC_P384, offCurve),
		"P-384 zero point":        tagged(AsymAlgo_ECC_P384, make([]byte, 96)),
		"short P-384 key":         tagged(AsymAlgo_ECC_P384, p384[:95]),
		"P-384 key as P-256":      tagged(AsymAlgo_ECC_P256, p384),
		"P-256 half as P-384":     tagged(AsymAlgo_ECC_P384, p384[:64]),
		"even modulus":            tagged(AsymAlgo_RSA2048, even),
		"modulus of 2047 bits":    tagged(AsymAlgo_RSA2048, short),
		"RSA-2048 key as 3072":    tagged(AsymAlgo_RSA3072, rsa2048),
		"RSA-3072 key as 2048":    tagged(AsymAlgo_RSA2048, rsa3072),
		"RSA-2048 key padded":     tagged(AsymAlgo_RSA3072, append(make([]byte, 128), rsa2048...)),
		"RSA-2048 key as P-384":   tagged(AsymAlgo_ECC_P384, rsa2048[:96]),
		"RSA-2048 key as ML-DSA":  tagged(AsymAlgo_MLDSA65, rsa2048),
		"empty RSA-2048 modulus":  tagged(AsymAlgo_RSA2048, nil),
		"empty P-384 coordinates": tagged(AsymAlgo_ECC_P384, nil),
	}
	for name, enc := range cases {
		if key, err := ParseAccountKey(enc); !errors.Is(err, ErrAccountKeyFormat) {
			t.Errorf("%s: parsed as %v (%v)", name, key, err)
		}
	}
	// A private key of the other size, or not a key at all, is no private key of the algorithm
	if _, err := NewAsymAlgo(AsymAlgo_RSA2048).PrivateToPublic(keys[AsymAlgo_RSA3072].priv); err == nil {
		t.Error("an RSA-3072 private key is an RSA-2048 key")
	}
	for _, a := range pivAlgos {
		if _, err := NewAsymAlgo(a).Sign([]byte{1, 2, 3}, Hash([]byte("x"))); err == nil {
			t.Errorf("%s signed with a malformed private key", a)
		}
	}
}

// Each RSA algorithm has one signature format: RSASSA-PSS with the network hash and a salt as
// long as the hash. PKCS #1 v1.5 signatures, other salt lengths and other hashes are refused.
func TestRSASignatureFormatIsPSSOnly(t *testing.T) {
	keys := pivKeys(t)
	priv, err := x509.ParsePKCS1PrivateKey(keys[AsymAlgo_RSA2048].priv)
	if err != nil {
		t.Fatal(err)
	}
	pub := keys[AsymAlgo_RSA2048].pub
	asym := &asymalgo.RSA{Bits: 2048, Hash: stdcrypto.SHA256}
	digest := NewHashAlgo(HashAlgo_SHA256).GetHash([]byte("transfer"))
	// A salt length of 0 is rsa.PSSSaltLengthAuto, so the shortest explicit salt is 1 byte
	pss := func(salt int, h stdcrypto.Hash, d []byte) []byte {
		sig, err := rsa.SignPSS(rand.Reader, priv, h, d, &rsa.PSSOptions{SaltLength: salt, Hash: h})
		if err != nil {
			t.Fatal(err)
		}
		return sig
	}
	if ok, err := asym.Verify(pub, digest, pss(rsa.PSSSaltLengthEqualsHash, stdcrypto.SHA256, digest)); !ok || err != nil {
		t.Fatalf("control: %v", err)
	}
	pkcs1, err := rsa.SignPKCS1v15(rand.Reader, priv, stdcrypto.SHA256, digest)
	if err != nil {
		t.Fatal(err)
	}
	sha384 := NewHashAlgo(HashAlgo_SHA384).GetHash([]byte("transfer"))
	refused := map[string][]byte{
		"PKCS #1 v1.5":           pkcs1,
		"salt of 1 byte":         pss(1, stdcrypto.SHA256, digest),
		"salt of 20 bytes":       pss(20, stdcrypto.SHA256, digest),
		"salt of 31 bytes":       pss(31, stdcrypto.SHA256, digest),
		"SHA-384 PSS on SHA-256": pss(rsa.PSSSaltLengthEqualsHash, stdcrypto.SHA384, sha384),
	}
	// FIPS 140-only mode refuses to make a salt longer than the hash
	if long, err := rsa.SignPSS(rand.Reader, priv, stdcrypto.SHA256, digest, &rsa.PSSOptions{SaltLength: 64}); err == nil {
		refused["salt of 64 bytes"] = long
	}
	for name, sig := range refused {
		if ok, _ := asym.Verify(pub, digest, sig); ok {
			t.Errorf("%s verified", name)
		}
	}
	other := &asymalgo.RSA{Bits: 3072, Hash: stdcrypto.SHA256}
	if ok, _ := other.Verify(keys[AsymAlgo_RSA3072].pub, digest, pss(rsa.PSSSaltLengthEqualsHash, stdcrypto.SHA256, digest)); ok {
		t.Error("an RSA-2048 signature verified with an RSA-3072 key")
	}
}

// The PIV algorithms are account keys only: no node signs with them, FIPS or not; a FIPS node
// verifies them
func TestPIVAlgosAreAccountOnly(t *testing.T) {
	for _, a := range accountAlgos {
		for _, fips := range []bool{true, false} {
			err := checkNodeAlgo(a, fips, true, "latest")
			if want := !slices.Contains(pivAlgos, a) && checkAsymAlgo(a, fips, true, "latest") == nil; (err == nil) != want {
				t.Errorf("%s fips=%v as node algorithm: %v", a, fips, err)
			}
		}
	}
	for _, a := range pivAlgos {
		if err := checkAsymAlgo(a, true, true, "latest"); err != nil {
			t.Errorf("%s refused as an account algorithm in FIPS mode: %v", a, err)
		}
	}
}

// RSA accounts need a hash RSASSA-PSS is defined with; the other account algorithms do not
func TestAccountAlgoNetwork(t *testing.T) {
	for _, h := range hashAlgos {
		withHash(h, func() {
			for _, a := range accountAlgos {
				err := CheckAccountAlgoNetwork(a)
				if want := !IsRSAAlgo(a) || PSSHash(h) != 0; (err == nil) != want {
					t.Errorf("%s on %s: %v", a, h, err)
				}
			}
		})
	}
}
