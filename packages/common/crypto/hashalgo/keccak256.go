package hashalgo

import (
	"crypto/hmac"
	"golang.org/x/crypto/sha3"
)

type Keccak256 struct{}

func (k *Keccak256) GetHMAC(secret string, message string) ([]byte, error) {
	mac := hmac.New(sha3.NewLegacyKeccak256, []byte(secret))
	mac.Write([]byte(message))
	return mac.Sum(nil), nil
}

func (k *Keccak256) GetHash(msg []byte) []byte {
	return k.usingKeccak256(msg)
}

func (k *Keccak256) DoubleHash(msg []byte) []byte {
	return k.usingKeccak256(k.usingKeccak256(msg))
}

func (k *Keccak256) usingKeccak256(data []byte) []byte {
	d := sha3.NewLegacyKeccak256()
	d.Write(data)
	return d.Sum(nil)
}
