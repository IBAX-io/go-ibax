package hashalgo

import (
	"crypto/hmac"
	"crypto/sha512"
)

type SHA384 struct{}

func (s *SHA384) Size() int { return sha512.Size384 }

func (s *SHA384) GetHMAC(secret string, message string) ([]byte, error) {
	mac := hmac.New(sha512.New384, []byte(secret))
	mac.Write([]byte(message))
	return mac.Sum(nil), nil
}

func (s *SHA384) GetHash(msg []byte) []byte {
	return s.usingSha384(msg)
}

func (s *SHA384) DoubleHash(msg []byte) []byte {
	return s.usingSha384(s.usingSha384(msg))
}

func (s *SHA384) usingSha384(data []byte) []byte {
	out := sha512.Sum384(data)
	return out[:]
}
