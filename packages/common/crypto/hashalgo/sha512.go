package hashalgo

import (
	"crypto/hmac"
	"crypto/sha512"
)

type SHA512 struct{}

func (s *SHA512) Size() int { return sha512.Size }

func (s *SHA512) GetHMAC(secret string, message string) ([]byte, error) {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(message))
	return mac.Sum(nil), nil
}

func (s *SHA512) GetHash(msg []byte) []byte {
	return s.usingSha512(msg)
}

func (s *SHA512) DoubleHash(msg []byte) []byte {
	return s.usingSha512(s.usingSha512(msg))
}

func (s *SHA512) usingSha512(data []byte) []byte {
	out := sha512.Sum512(data)
	return out[:]
}
