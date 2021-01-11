package crypto

import (
	"encoding/hex"
	"fmt"
)

type Cryptoer interface {
	genKeyPair() ([]byte, []byte, error)
	sign(privateKey, data []byte) ([]byte, error)
	verify(public, data, signature []byte) (bool, error)
	privateToPublic(key []byte) ([]byte, error)
}

type Oval struct {
	name string
}

const (
	cSM2   = "SM2"
	cECDSA = "ECDSA"
)

var Curve = &curve

var curve Oval

func InitCurve(s string) {
	switch s {
	case cECDSA:
		curve.name = cECDSA
		return
	case cSM2:
		curve.name = cSM2
		return
	}
	panic(fmt.Errorf("curve [%v] is not supported yet", s))
}

func (o Oval) String() string {
	return o.name
}

func getCryptoer() Cryptoer {
	switch curve.name {
	case cSM2:
		return &SM2{}
	case cECDSA:
	}
	return hex.EncodeToString(priv), PubToHex(pub), nil
}

func Sign(privateKey, data []byte) ([]byte, error) {
	return getCryptoer().sign(privateKey, data)
}

func CheckSign(public, data, signature []byte) (bool, error) {
	return getCryptoer().verify(public, data, signature)
}

// PrivateToPublic returns the public key for the specified private key.
func PrivateToPublic(key []byte) ([]byte, error) {
	return getCryptoer().privateToPublic(key)
}
