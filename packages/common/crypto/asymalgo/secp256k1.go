package asymalgo

import (
	"crypto/ecdsa"

	crand "crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/common/gencache"
	"github.com/IBAX-io/go-ibax/packages/consts"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	secpecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

type Secp256k1 struct{}

// secpKeys are the parsed public keys of the latest signers
var secpKeys = gencache.New[string, *secp.PublicKey](65536)

func secpKey(public []byte) (*secp.PublicKey, error) {
	if k, ok := secpKeys.Get(string(public)); ok {
		return k, nil
	}
	k, err := secp.ParsePubKey(append([]byte{4}, public...))
	if err != nil {
		return nil, ErrInvalidPublicKey
	}
	secpKeys.Put(string(public), k)
	return k, nil
}

func (s *Secp256k1) GenKeyPair() ([]byte, []byte, error) {
	priv, err := ecdsa.GenerateKey(secp.S256(), crand.Reader)
	if err != nil {
		return nil, nil, err
	}
	pub := append(FillLeft(priv.X.Bytes()), FillLeft(priv.Y.Bytes())...)
	return FillLeft(priv.D.Bytes()), pub, nil
}

func (s *Secp256k1) Sign(privateKey, hash []byte) ([]byte, error) {
	if len(hash) == 0 {
		return nil, ErrSigningEmpty
	}
	pubkeyCurve := secp.S256()
	bi, err := privateScalar(privateKey, pubkeyCurve.Params().N)
	if err != nil {
		return nil, err
	}
	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = pubkeyCurve
	priv.D = bi
	priv.PublicKey.X, priv.PublicKey.Y = pubkeyCurve.ScalarBaseMult(bi.Bytes())
	r, s_, err := ecdsa.Sign(crand.Reader, priv, hash)
	if err != nil {
		return nil, err
	}
	ret := append(FillLeft(r.Bytes()), FillLeft(s_.Bytes())...)
	return ret, nil

}

func (s *Secp256k1) Verify(public, hash, signature []byte) (bool, error) {
	if len(public) == 0 {
		return false, ErrCheckingSignEmpty
	}
	if len(hash) == 0 {
		return false, fmt.Errorf("invalid parameters len(data) == 0")
	}
	if len(public) != consts.PubkeySizeLength {
		return false, fmt.Errorf("invalid parameters len(public) = %d", len(public))
	}
	if len(signature) == 0 {
		return false, fmt.Errorf("invalid parameters len(signature) == 0")
	}
	pubkey, err := secpKey(public)
	if err != nil {
		return false, err
	}
	r, s_, err := ParseSign(hex.EncodeToString(signature))
	if err != nil {
		return false, err
	}
	// ecdsa.Verify of the standard library: r and s in [1, N-1], the hash truncated to its first
	// 32 bytes
	var rs, ss secp.ModNScalar
	if r.Sign() <= 0 || s_.Sign() <= 0 || rs.SetByteSlice(r.Bytes()) || ss.SetByteSlice(s_.Bytes()) {
		return false, ErrIncorrectSign
	}
	if !secpecdsa.NewSignature(&rs, &ss).Verify(hash, pubkey) {
		return false, ErrIncorrectSign
	}
	return true, nil
}

func (s *Secp256k1) PrivateToPublic(key []byte) ([]byte, error) {
	priv := secp.PrivKeyFromBytes(key)
	return priv.PubKey().SerializeUncompressed()[1:], nil
}

func (s *Secp256k1) CheckPublicKey(public []byte) error {
	if len(public) != consts.PubkeySizeLength {
		return ErrInvalidPublicKey
	}
	if _, err := secp.ParsePubKey(append([]byte{4}, public...)); err != nil {
		return ErrInvalidPublicKey
	}
	return nil
}

func (e *Secp256k1) PublicKeySize() int {
	return consts.PubkeySizeLength
}

// SignatureSize is r || s, each left-padded to 32 bytes
func (e *Secp256k1) SignatureSize() int {
	return 64
}
