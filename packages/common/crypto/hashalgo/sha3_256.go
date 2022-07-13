package hashalgo

import (
	"crypto/hmac"

	"golang.org/x/crypto/sha3"
)

type Sha3256 struct{}

func (s *Sha3256) GetHMAC(secret string, message string) ([]byte, error) {
	mac := hmac.New(sha3.New256, []byte(secret))
	mac.Write([]byte(message))
	return mac.Sum(nil), nil
}

func (s *Sha3256) GetHash(msg []byte) []byte {
	return s.usingSha3(msg)
}

func (s *Sha3256) DoubleHash(msg []byte) []byte {
	return s.usingSha3(s.usingSha3(msg))
}

func (s *Sha3256) usingSha3(data []byte) []byte {
	hashed := sha3.Sum256(data)
	return hashed[:]
}
