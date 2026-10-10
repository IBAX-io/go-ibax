package crypto

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func TestAccountKeyRoundTrip(t *testing.T) {
	for _, a := range signAlgos {
		t.Run(a.String(), func(t *testing.T) {
			priv, key, err := GenAccountKey(a)
			if err != nil {
				t.Fatal(err)
			}
			enc := key.Bytes()
			if enc[0] != AccountKeyMarker || enc[1] != byte(a) || !bytes.Equal(enc[2:], key.Raw) {
				t.Fatalf("encoding %x", enc)
			}
			parsed, err := ParseAccountKey(enc)
			if err != nil || parsed.Algo != a || !bytes.Equal(parsed.Raw, key.Raw) {
				t.Fatalf("parse: %v %v", parsed, err)
			}
			fromHex, err := ParseAccountKeyHex(key.Hex())
			if err != nil || !bytes.Equal(fromHex.Bytes(), enc) {
				t.Fatalf("parse hex: %v", err)
			}
			if key.Address() != Address(key.Raw) {
				t.Fatal("the address is not that of the bare key")
			}
			derived, err := AccountKeyOf(a, priv)
			if err != nil || !bytes.Equal(derived.Bytes(), enc) {
				t.Fatalf("key of the private key: %v", err)
			}
			msg := []byte("transfer")
			sig, err := AccountSign(a, priv, msg)
			if err != nil {
				t.Fatal(err)
			}
			if ok, err := parsed.Verify(msg, sig); !ok || err != nil {
				t.Fatalf("verify: %v", err)
			}
			if ok, _ := parsed.Verify([]byte("transfer!"), sig); ok {
				t.Fatal("verified for another message")
			}
			_, other, err := GenAccountKey(a)
			if err != nil {
				t.Fatal(err)
			}
			if ok, _ := other.Verify(msg, sig); ok {
				t.Fatal("verified with another key of the algorithm")
			}
		})
	}
}

func TestAccountKeyRefusesMalformed(t *testing.T) {
	_, p256, err := GenAccountKey(AsymAlgo_ECC_P256)
	if err != nil {
		t.Fatal(err)
	}
	_, mldsa, err := GenAccountKey(AsymAlgo_MLDSA65)
	if err != nil {
		t.Fatal(err)
	}
	tagged := func(algo byte, raw []byte) []byte { return append([]byte{AccountKeyMarker, algo}, raw...) }
	offCurve := bytes.Clone(p256.Raw)
	offCurve[63] ^= 1
	coordinates := bytes.Repeat([]byte{0xff}, 64)
	cases := map[string][]byte{
		"nil":                     nil,
		"marker only":             {AccountKeyMarker},
		"no public key":           {AccountKeyMarker, byte(AsymAlgo_ECC_P256)},
		"bare 04 curve key":       append([]byte{4}, p256.Raw...),
		"bare 64-byte curve key":  p256.Raw,
		"DER marker":              tagged(0x30, p256.Raw)[1:],
		"unknown algorithm":       tagged(99, p256.Raw),
		"unimplemented ECC_P512":  tagged(byte(AsymAlgo_ECC_P512), p256.Raw),
		"short curve key":         tagged(byte(AsymAlgo_ECC_P256), p256.Raw[:63]),
		"long curve key":          tagged(byte(AsymAlgo_ECC_P256), append(bytes.Clone(p256.Raw), 0)),
		"P-256 point off curve":   tagged(byte(AsymAlgo_ECC_P256), offCurve),
		"P-256 zero point":        tagged(byte(AsymAlgo_ECC_P256), make([]byte, 64)),
		"secp256k1 coordinates":   tagged(byte(AsymAlgo_ECC_Secp256k1), coordinates),
		"SM2 coordinates":         tagged(byte(AsymAlgo_SM2), coordinates),
		"short ML-DSA-65 key":     tagged(byte(AsymAlgo_MLDSA65), mldsa.Raw[:len(mldsa.Raw)-1]),
		"ML-DSA-65 key as 87":     tagged(byte(AsymAlgo_MLDSA87), mldsa.Raw),
		"P-256 key as ML-DSA-65":  tagged(byte(AsymAlgo_MLDSA65), p256.Raw),
		"ML-DSA-65 key as P-256":  tagged(byte(AsymAlgo_ECC_P256), mldsa.Raw),
		"ML-DSA-65 key truncated": tagged(byte(AsymAlgo_MLDSA65), mldsa.Raw[:64]),
	}
	for name, enc := range cases {
		if key, err := ParseAccountKey(enc); !errors.Is(err, ErrAccountKeyFormat) {
			t.Errorf("%s: parsed as %v (%v)", name, key, err)
		}
	}
	for _, s := range []string{"", "zz", "ac00"} {
		if _, err := ParseAccountKeyHex(s); !errors.Is(err, ErrAccountKeyFormat) {
			t.Errorf("hex %q: %v", s, err)
		}
	}
	if _, _, err := GenAccountKey(AsymAlgo_ECC_P512); err == nil {
		t.Error("generated an ECC_P512 key")
	}
	if _, err := AccountSign(AsymAlgo(99), nil, []byte("x")); err == nil {
		t.Error("signed with an unknown algorithm")
	}
}

// A key relabelled with another algorithm must never verify the signatures of the original: the
// algorithm comes from the key, so a relabelled key either does not parse or verifies nothing.
// The control verifies the same signature with the original key.
func TestAccountKeyRelabelledDoesNotVerify(t *testing.T) {
	msg := []byte("transfer")
	for _, a := range signAlgos {
		priv, key, err := GenAccountKey(a)
		if err != nil {
			t.Fatal(err)
		}
		sig, err := AccountSign(a, priv, msg)
		if err != nil {
			t.Fatal(err)
		}
		if ok, err := key.Verify(msg, sig); !ok || err != nil {
			t.Fatalf("%s: control: %v", a, err)
		}
		for _, b := range signAlgos {
			if b == a {
				continue
			}
			t.Run(fmt.Sprintf("%s as %s", a, b), func(t *testing.T) {
				enc := key.Bytes()
				enc[1] = byte(b)
				relabelled, err := ParseAccountKey(enc)
				if err != nil {
					return
				}
				if ok, _ := relabelled.Verify(msg, sig); ok {
					t.Fatal("a relabelled key verified the signature")
				}
			})
		}
	}
	// P-256 and secp256k1 keys are both 64 bytes: only the algorithm byte tells them apart. A
	// point on both curves is unlikely, so the relabelled key is checked directly with the
	// provider as well.
	priv, key, err := GenAccountKey(AsymAlgo_ECC_P256)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := AccountSign(AsymAlgo_ECC_P256, priv, msg)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := NewAsymAlgo(AsymAlgo_ECC_Secp256k1).Verify(key.Raw, Hash(msg), sig); ok {
		t.Fatal("a P-256 signature verified as secp256k1")
	}
}
