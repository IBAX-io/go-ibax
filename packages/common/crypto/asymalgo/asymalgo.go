package asymalgo

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
)

// ParseSign converts the hex signature to r and s big number
func ParseSign(sign string) (*big.Int, *big.Int, error) {
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

// FillLeft is filling slice
func FillLeft(slice []byte) []byte {
	if len(slice) >= 32 {
		return slice
	}
	return append(make([]byte, 32-len(slice)), slice...)
}
