package hashalgo

import (
	"crypto/hmac"

	"github.com/tjfoc/gmsm/sm3"
)

type SM3 struct{}

func (s *SM3) GetHMAC(secret string, message string) ([]byte, error) {
	mac := hmac.New(sm3.New, []byte(secret))
	mac.Write([]byte(message))
	return mac.Sum(nil), nil
}

func (s *SM3) GetHash(msg []byte) []byte {
	return s.usingSM3(msg)
}

func (s *SM3) DoubleHash(msg []byte) []byte {
	return s.usingSM3(s.usingSM3(msg))
}

func (s *SM3) usingSM3(data []byte) []byte {
	return sm3.Sm3Sum(data)
}
