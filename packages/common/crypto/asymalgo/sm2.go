package asymalgo

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/tjfoc/gmsm/sm2"
)

type SM2 struct{}

func (s *SM2) GenKeyPair() ([]byte, []byte, error) {
	priv, err := sm2.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return priv.D.Bytes(), append(FillLeft(priv.PublicKey.X.Bytes()), FillLeft(priv.PublicKey.Y.Bytes())...), nil
}

func (s *SM2) Sign(privateKey, hash []byte) ([]byte, error) {
	if len(hash) == 0 {
		return nil, ErrSigningEmpty
	}
	pubkeyCurve := sm2.P256Sm2()
	bi, err := privateScalar(privateKey, pubkeyCurve.Params().N)
	if err != nil {
		return nil, err
	}
	priv := new(sm2.PrivateKey)
	priv.PublicKey.Curve = pubkeyCurve
	priv.D = bi
	priv.PublicKey.X, priv.PublicKey.Y = pubkeyCurve.ScalarBaseMult(bi.Bytes())
	return priv.Sign(rand.Reader, hash, nil)
}

func (s *SM2) Verify(public, hash, signature []byte) (bool, error) {
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
	pubkey := new(sm2.PublicKey)
	pubkey.Curve = sm2.P256Sm2()
	pubkey.X = new(big.Int).SetBytes(public[0:consts.PrivkeyLength])
	pubkey.Y = new(big.Int).SetBytes(public[consts.PrivkeyLength:])
	verify := pubkey.Verify(hash, signature)
	if !verify {
		return false, ErrIncorrectSign
	}
	return true, nil
}

func (s *SM2) PrivateToPublic(key []byte) ([]byte, error) {
	pubkeyCurve := sm2.P256Sm2()
	bi := new(big.Int).SetBytes(key)
	priv := new(sm2.PrivateKey)
	priv.PublicKey.Curve = pubkeyCurve
	priv.D = bi
	priv.PublicKey.X, priv.PublicKey.Y = pubkeyCurve.ScalarBaseMult(key)
	return append(FillLeft(priv.PublicKey.X.Bytes()), FillLeft(priv.PublicKey.Y.Bytes())...), nil
}

func (e *SM2) PublicKeySize() int {
	return consts.PubkeySizeLength
}

// SignatureSize is the longest ASN.1 DER signature: a sequence of two 33-byte integers
func (e *SM2) SignatureSize() int {
	return 72
}
