package asymalgo

import (
	"crypto/ecdsa"

	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/IBAX-io/go-ibax/packages/consts"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

type Secp256k1 struct{}

func (s *Secp256k1) GenKeyPair() ([]byte, []byte, error) {
	priv, err := ecdsa.GenerateKey(secp.S256(), crand.Reader)
	if err != nil {
		return nil, nil, err
	}
	pub := append(FillLeft(priv.X.Bytes()), FillLeft(priv.Y.Bytes())...)
	return priv.D.Bytes(), pub, nil
}

func (s *Secp256k1) Sign(privateKey, hash []byte) ([]byte, error) {
	if len(hash) == 0 {
		return nil, ErrSigningEmpty
	}
	pubkeyCurve := secp.S256()
	bi := new(big.Int).SetBytes(privateKey)
	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = pubkeyCurve
	priv.D = bi
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
	pubkeyCurve := secp.S256()
	pubkey := new(ecdsa.PublicKey)
	pubkey.Curve = pubkeyCurve
	pubkey.X = new(big.Int).SetBytes(public[0:consts.PrivkeyLength])
	pubkey.Y = new(big.Int).SetBytes(public[consts.PrivkeyLength:])
	r, s_, err := ParseSign(hex.EncodeToString(signature))
	if err != nil {
		return false, err
	}
	verify := ecdsa.Verify(pubkey, hash, r, s_)
	if !verify {
		return false, ErrIncorrectSign
	}
	return true, nil
}

func (s *Secp256k1) PrivateToPublic(key []byte) ([]byte, error) {
	priv := secp.PrivKeyFromBytes(key)
	return priv.PubKey().SerializeUncompressed()[1:], nil
}
