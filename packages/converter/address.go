/*----------------------------------------------------------------
- Copyright (c) IBAX. All rights reserved.
- See LICENSE in the project root for license information.
---------------------------------------------------------------*/

package converter

import (
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"
)

type Hole struct {
	K int64
	S string
}

const (
	BlackHoleAddr = "BlackHole"
	WhiteHoleAddr = "WhiteHole"
)

var (
	HoleAddrMap = map[string]Hole{
		BlackHoleAddr: {K: 0, S: "0000-0000-0000-0000-0000"},
		WhiteHoleAddr: {K: 5555, S: "0000-0000-0000-0000-5555"},
	}
)

// AddressToID converts the string representation of the wallet number to a numeric
func AddressToID(input string) (addr int64) {
	input = strings.TrimSpace(input)
	if len(input) < 2 {
		return 0
	}
	if input[0] == '-' {
		uaddr, err := strconv.ParseInt(input, 10, 64)
		if err != nil {

		}
		addr = uaddr
	} else if has4LineContain(input) {
		addr = StringToAddress(input)
	} else {
		uaddr, err := strconv.ParseUint(input, 10, 64)
		if err != nil {

		}
		addr = int64(uaddr)
	}
	if IDToAddress(addr) == `invalid` {
		return 0
	}
	return
}

// IDToAddress converts the identifier of account to a string of the form xxxx-xxxx-xxxx-xxxx-xxxx.
func IDToAddress(id int64) (out string) {
	out = AddressToString(id)
	if !IsValidAddress(out) {
		out = `invalid`
	}
	return
}

// AddressToString converts int64 address to chain address as xxxx-xxxx-xxxx-xxxx-xxxx.
func AddressToString(int int64) (str string) {
	return AddressToStringUint64(uint64(int))
}

func AddressToStringUint64(uint uint64) (str string) {
	num := strconv.FormatUint(uint, 10)
	val := []byte(strings.Repeat("0", consts.AddressLength-len(num)) + num)
	for i := 0; i <= 4; i++ {
		if i == 4 {
			str += string(val[i*4:])
			break
		}
		str += string(val[i*4:(i+1)*4]) + `-`
	}
	return
}

// StringToAddress converts string chain address to int64 address. The input address can be a positive or negative
// number, or chain address in xxxx-xxxx-xxxx-xxxx-xxxx format. Returns 0 when error occurs.
func StringToAddress(str string) (result int64) {
	var (
		err error
		ret uint64
	)
	if len(str) == 0 {
		return 0
	}
	//string of int64
	if str[0] == '-' {
		var id int64
		id, err = strconv.ParseInt(str, 10, 64)
		if err != nil {
			return 0
		}
		str = strconv.FormatUint(uint64(id), 10)
	}
	if len(str) < consts.AddressLength {
		str = strings.Repeat(`0`, consts.AddressLength-len(str)) + str
	}
	val := []byte(strings.Replace(str, `-`, ``, -1))
	if len(val) != consts.AddressLength {
		return 0
	}
	if ret, err = strconv.ParseUint(string(val), 10, 64); err != nil {
		return 0
	}
	if CheckSum(val[:len(val)-1]) != int(val[len(val)-1]-'0') {
		return 0
	}
	result = int64(ret)
	return
}

// IsValidAddress checks if the specified address is chain address.
func IsValidAddress(address string) bool {
	val := []byte(strings.Replace(address, `-`, ``, -1))
	if len(val) != consts.AddressLength {
		return false
	}
	if _, err := strconv.ParseUint(string(val), 10, 64); err != nil {
		return false
	}
	return CheckSum(val[:len(val)-1]) == int(val[len(val)-1]-'0')
}

// CheckSum calculates the 0-9 check sum of []byte
func CheckSum(val []byte) int {
	var one, two int
	for i, ch := range val {
		digit := int(ch - '0')
		if i&1 == 1 {
			one += digit
		} else {
			two += digit
		}
	}
	checksum := (two + 3*one) % 10
	if checksum > 0 {
		checksum = 10 - checksum
	}
	return checksum
}

func has4LineContain(str string) bool {
	return strings.Count(str, "-") == 4
}
