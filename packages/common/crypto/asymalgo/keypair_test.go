package asymalgo

import (
	"bytes"
	"testing"
)

type keyPairGenerator interface {
	GenKeyPair() ([]byte, []byte, error)
	PrivateToPublic(key []byte) ([]byte, error)
}

// One scalar in 256 has a zero first byte, so 4096 keys hold a short one but with a chance of
// (255/256)^4096, about 1e-7: every private key is written in 32 bytes, as clients read it
func TestGenKeyPairWritesThirtyTwoBytes(t *testing.T) {
	for name, algorithm := range map[string]keyPairGenerator{
		"P256":      &P256{},
		"Secp256k1": &Secp256k1{},
		"SM2":       &SM2{},
	} {
		t.Run(name, func(t *testing.T) {
			for i := 0; i < 4096; i++ {
				private, public, err := algorithm.GenKeyPair()
				if err != nil {
					t.Fatal(err)
				}
				if len(private) != 32 {
					t.Fatalf("private key of %d bytes: %x", len(private), private)
				}
				derived, err := algorithm.PrivateToPublic(private)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(derived, public) {
					t.Fatalf("the public key of %x is not the one generated with it", private)
				}
			}
		})
	}
}
