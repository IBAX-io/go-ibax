package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"

	"github.com/tjfoc/gmsm/sm3"
)

type Hasher interface {
	// GetHMAC returns HMAC hash
	getHMAC(secret string, message string) ([]byte, error)
	// Hash returns hash of passed bytes
	hash(msg []byte) []byte
	// DoubleHash returns double hash of passed bytes
	doubleHash(msg []byte) []byte
}

type Hval struct {
	name string
}

const (
	hSM3    = "SM3"
	hSHA256 = "SHA256"
)

var hal Hval
var Hal = &hal

func (h Hval) String() string {
	return h.name
}

func getHasher() Hasher {
	switch hal.name {
	case hSM3:
		return &SM3{}
	case hSHA256:
		return &SHA256{}
	default:
		panic(fmt.Errorf("hash is not supported yet or empty"))
	}
}

func InitHash(s string) {
	switch s {
	case hSM3:
		hal.name = hSM3
		return
	case hSHA256:
		hal.name = hSHA256
		return
	}
	panic(fmt.Errorf("hash [%v] is not supported yet", s))
}

func GetHMAC(secret string, message string) ([]byte, error) {
	return getHasher().getHMAC(secret, message)
}

func Hash(msg []byte) []byte {
	return getHasher().hash(msg)
}

func DoubleHash(msg []byte) []byte {
	return getHasher().doubleHash(msg)
}

// Address gets int64 address from the public key
func Address(pubKey []byte) int64 {
	pubKey = CutPub(pubKey)
	h := getHasher().hash(pubKey)
	h512 := sha512.Sum512(h[:])
	crc := calcCRC64(h512[:])
	// replace the last digit by checksum
	num := strconv.FormatUint(crc, 10)
	val := []byte(strings.Repeat("0", consts.AddressLength-len(num)) + num)
	return int64(crc - (crc % 10) + uint64(checkSum(val[:len(val)-1])))
}

type SM3 struct {
	Hasher
}

type SHA256 struct {
	Hasher
}

func (s *SM3) getHMAC(secret string, message string) ([]byte, error) {
	mac := hmac.New(sm3.New, []byte(secret))
	mac.Write([]byte(message))
	return mac.Sum(nil), nil
}

func (s *SM3) hash(msg []byte) []byte {
	return sm3.Sm3Sum(msg)
}

func (s *SM3) doubleHash(msg []byte) []byte {
	return s.doubleSM3(msg)
}

func (s *SM3) doubleSM3(data []byte) []byte {
	return s.usingSM3(s.usingSM3(data))
}

func (s *SM3) usingSM3(data []byte) []byte {
	return sm3.Sm3Sum(data)
}

func (s *SHA256) getHMAC(secret string, message string) ([]byte, error) {
	return getHMAC(secret, message)
}
func (s *SHA256) hash(msg []byte) []byte {
	return s._Hash(msg)
}

func (s *SHA256) doubleHash(msg []byte) []byte {
	return s.doubleSha256(msg)
}

func (s *SHA256) doubleSha256(data []byte) []byte {
	return s.usingSha256(s.usingSha256(data))
}
func (s *SHA256) usingSha256(data []byte) []byte {
	out := sha256.Sum256(data)
	return out[:]
}
