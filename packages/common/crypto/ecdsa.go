package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
)

type ECDSA struct{}

type signProvider int

const (
	_ECDSA signProvider = iota
)

func (e *ECDSA) genKeyPair() ([]byte, []byte, error) {
	var curve elliptic.Curve
	switch ellipticSize {
	case elliptic256:
		curve = elliptic.P256()
	default:
		return nil, nil, ErrUnsupportedCurveSize
	}
	private, err := ecdsa.GenerateKey(curve, crand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return private.D.Bytes(), append(converter.FillLeft(private.PublicKey.X.Bytes()), converter.FillLeft(private.PublicKey.Y.Bytes())...), nil
}

func (e *ECDSA) sign(privateKey, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrSigningEmpty
	}
	switch signProv {
	case _ECDSA:
		return e.signECDSA(privateKey, data)
	default:
		return nil, ErrUnknownProvider
	}

}

func (e *ECDSA) privateToPublic(key []byte) ([]byte, error) {
	pubkeyCurve := elliptic.P256()
	bi := new(big.Int).SetBytes(key)
	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = pubkeyCurve
	priv.D = bi
	priv.PublicKey.X, priv.PublicKey.Y = pubkeyCurve.ScalarBaseMult(key)
	return append(converter.FillLeft(priv.PublicKey.X.Bytes()), converter.FillLeft(priv.PublicKey.Y.Bytes())...), nil
}

// checkSign is checking sign
func (e *ECDSA) verify(public, data, signature []byte) (bool, error) {
	if len(public) == 0 {
		return false, ErrCheckingSignEmpty
	}
	switch signProv {
	case _ECDSA:
		return e.checkECDSA(public, data, signature)
	default:
		return false, ErrUnknownProvider
	}
}

func (e *ECDSA) signECDSA(privateKey, data []byte) (ret []byte, err error) {
	pubkeyCurve := elliptic.P256()
	bi := new(big.Int).SetBytes(privateKey)
	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = pubkeyCurve
	priv.D = bi
	r, s, err := ecdsa.Sign(crand.Reader, priv, getHasher().hash(data))
	if err != nil {
		return
	}
	ret = append(converter.FillLeft(r.Bytes()), converter.FillLeft(s.Bytes())...)
	return
}

// CheckECDSA checks if forSign has been signed with corresponding to public the private key
func (e *ECDSA) checkECDSA(public, data, signature []byte) (bool, error) {
	if len(data) == 0 {
		return false, fmt.Errorf("invalid parameters len(data) == 0")
	}
	if len(public) != consts.PubkeySizeLength {
		return false, fmt.Errorf("invalid parameters len(public) = %d", len(public))
	}
	if len(signature) == 0 {
		return false, fmt.Errorf("invalid parameters len(signature) == 0")
	}

	var pubkeyCurve elliptic.Curve
	switch ellipticSize {
	case elliptic256:
		pubkeyCurve = elliptic.P256()
	default:
		return false, ErrUnsupportedCurveSize
	}
	pubkey := new(ecdsa.PublicKey)
	pubkey.Curve = pubkeyCurve
	pubkey.X = new(big.Int).SetBytes(public[0:consts.PrivkeyLength])
	pubkey.Y = new(big.Int).SetBytes(public[consts.PrivkeyLength:])
	r, s, err := e.parseSign(hex.EncodeToString(signature))
	if err != nil {
		return false, err
	}
	verifystatus := ecdsa.Verify(pubkey, getHasher().hash(data), r, s)
	if !verifystatus {
		return false, ErrIncorrectSign
	}
	return true, nil
}

// parseSign converts the hex signature to r and s big number
func (e *ECDSA) parseSign(sign string) (*big.Int, *big.Int, error) {
	var (
		binSign []byte
		err     error
	)
	//	var off int
	parse := func(bsign []byte) []byte {
		blen := int(bsign[1])
		if blen > len(bsign)-2 {
			return nil
		}
		ret := bsign[2 : 2+blen]
		if len(ret) > 32 {
			ret = ret[len(ret)-32:]
		} else if len(ret) < 32 {
			ret = append(bytes.Repeat([]byte{0}, 32-len(ret)), ret...)
		}
		return ret
	}
	if len(sign) > 128 {
		binSign, err = hex.DecodeString(sign)
		if err != nil {
			return nil, nil, fmt.Errorf("decoding sign from string: %w", err)
		}
		left := parse(binSign[2:])
		if left == nil || int(binSign[3])+6 > len(binSign) {
			return nil, nil, fmt.Errorf(`wrong left parsing`)
		}
		right := parse(binSign[4+binSign[3]:])
		if right == nil {
			return nil, nil, fmt.Errorf(`wrong right parsing`)
		}
		sign = hex.EncodeToString(append(left, right...))
	} else if len(sign) < 128 {
		return nil, nil, fmt.Errorf(`wrong len of signature %d`, len(sign))
	}
	all, err := hex.DecodeString(sign[:])
	if err != nil {
		return nil, nil, fmt.Errorf("wrong signature size: %w", err)
	}
	return new(big.Int).SetBytes(all[:32]), new(big.Int).SetBytes(all[len(all)-32:]), nil
}
