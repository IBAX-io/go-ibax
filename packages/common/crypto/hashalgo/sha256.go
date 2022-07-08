package hashalgo

import (
	"crypto/hmac"
	"crypto/sha256"
)

type SHA256 struct{}

func (s *SHA256) GetHMAC(secret string, message string) ([]byte, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return mac.Sum(nil), nil
}
func (s *SHA256) GetHash(msg []byte) []byte {
	return s.usingSha256(msg)
}

func (s *SHA256) DoubleHash(msg []byte) []byte {
	return s.usingSha256(s.usingSha256(msg))
}

func (s *SHA256) usingSha256(data []byte) []byte {
	out := sha256.Sum256(data)
	return out[:]
}
