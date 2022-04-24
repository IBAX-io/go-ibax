package asymalgo

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/IBAX-io/go-ibax/packages/consts"
)

type P256 struct{}

func (e *P256) GenKeyPair() ([]byte, []byte, error) {
	var curve elliptic.Curve
	curve = elliptic.P256()
	private, err := ecdsa.GenerateKey(curve, crand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return private.D.Bytes(), append(FillLeft(private.PublicKey.X.Bytes()), FillLeft(private.PublicKey.Y.Bytes())...), nil
}

func (e *P256) Sign(privateKey, hash []byte) ([]byte, error) {
	if len(hash) == 0 {
		return nil, ErrSigningEmpty
	}
	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = elliptic.P256()
	priv.D = new(big.Int).SetBytes(privateKey)
	r, s, err := ecdsa.Sign(crand.Reader, priv, hash)
	if err != nil {
		return nil, err
	}
	return append(FillLeft(r.Bytes()), FillLeft(s.Bytes())...), nil
}

func (e *P256) Verify(public, hash, signature []byte) (bool, error) {
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

	pubkey := new(ecdsa.PublicKey)
	pubkey.Curve = elliptic.P256()
	pubkey.X = new(big.Int).SetBytes(public[0:consts.PrivkeyLength])
	pubkey.Y = new(big.Int).SetBytes(public[consts.PrivkeyLength:])
	r, s, err := ParseSign(hex.EncodeToString(signature))
	if err != nil {
		return false, err
	}
	verify := ecdsa.Verify(pubkey, hash, r, s)
	if !verify {
		return false, ErrIncorrectSign
	}
	return true, nil
}

func (e *P256) PrivateToPublic(key []byte) ([]byte, error) {
	pubkeyCurve := elliptic.P256()
	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = pubkeyCurve
	priv.D = new(big.Int).SetBytes(key)
	priv.PublicKey.X, priv.PublicKey.Y = pubkeyCurve.ScalarBaseMult(key)
	return append(FillLeft(priv.PublicKey.X.Bytes()), FillLeft(priv.PublicKey.Y.Bytes())...), nil
}
