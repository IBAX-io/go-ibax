/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package converter

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/IBAX-io/go-ibax/packages/consts"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

var ErrSliceSize = errors.New("Slice size larger than buffer size")

var FirstEcosystemTables = map[string]bool{
	`keys`:               true,
	`menu`:               true,
	`pages`:              true,
	`snippets`:           true,
	`languages`:          true,
	`contracts`:          true,
	`tables`:             true,
	`parameters`:         true,
	`history`:            true,
	`sections`:           true,
	`members`:            true,
	`roles`:              true,
	`roles_participants`: true,
	`notifications`:      true,
	`applications`:       true,
	`binaries`:           true,
	`buffer_data`:        true,
	`app_params`:         true,
	`views`:              true,
}

func EncodeLenInt64(data *[]byte, x int64) *[]byte {
	var length int
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(x))
	for length = 8; length > 0 && buf[length-1] == 0; length-- {
	}
	*data = append(append(*data, byte(length)), buf[:length]...)
	return data
}

func EncodeLenInt64InPlace(x int64) []byte {
	buf := make([]byte, 9)
	value := buf[1:]
	binary.LittleEndian.PutUint64(value, uint64(x))
	var length byte
	for length = 8; length > 0 && value[length-1] == 0; length-- {
	}
	buf[0] = length
	return buf[:length+1]
}

func EncodeLenByte(out *[]byte, buf []byte) *[]byte {
	*out = append(append(*out, EncodeLength(int64(len(buf)))...), buf...)
	return out
}

// EncodeLength encodes int64 number to []byte. If it is less than 128 then it returns []byte{length}.
// Otherwise, it returns (0x80 | len of int64) + int64 as BigEndian []byte
//
//	67 => 0x43
//	1024 => 0x820400
//	1000000 => 0x830f4240
func EncodeLength(length int64) []byte {
	if length >= 0 && length <= 127 {
		return []byte{byte(length)}
	}
	buf := make([]byte, 9)
	binary.BigEndian.PutUint64(buf[1:], uint64(length))
	i := 1
	for ; buf[i] == 0 && i < 8; i++ {
	}
	buf[0] = 0x80 | byte(9-i)
	return append(buf[:1], buf[i:]...)
}

// DecodeLenInt64 gets int64 from []byte and shift the slice. The []byte should  be
// encoded with EncodeLengthPlusInt64.
func DecodeLenInt64(data *[]byte) (int64, error) {
	if len(*data) == 0 {
		return 0, nil
	}
	length := int((*data)[0]) + 1
	if len(*data) < length {
		log.WithFields(log.Fields{"data_length": len(*data), "length": length, "type": consts.UnmarshallingError}).Error("length of data is smaller then encoded length")
		return 0, fmt.Errorf(`length of data %d < %d`, len(*data), length)
	}
	buf := make([]byte, 8)
	copy(buf, (*data)[1:length])
	x := int64(binary.LittleEndian.Uint64(buf))
	*data = (*data)[length:]
	return x, nil
}

func DecodeLenInt64Buf(buf *bytes.Buffer) (int64, error) {
	if buf.Len() == 0 {
		return 0, nil
	}

	val, err := buf.ReadByte()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("cannot read bytes from buffer")
		return 0, err
	}

	length := int(val)
	if buf.Len() < length {
		log.WithFields(log.Fields{"type": consts.UnmarshallingError, "data_length": buf.Len(), "length": length}).Error("length of data is smaller then encoded length")
		return 0, fmt.Errorf(`length of data %d < %d`, buf.Len(), length)
	}
	data := make([]byte, 8)
	copy(data, buf.Next(length))

	return int64(binary.LittleEndian.Uint64(data)), nil

}

// DecodeLength decodes []byte to int64 and shifts buf. Bytes must be encoded with EncodeLength function.
//
//	0x43 => 67
//	0x820400 => 1024
//	0x830f4240 => 1000000
func DecodeLength(buf *[]byte) (ret int64, err error) {
	if len(*buf) == 0 {
		return
	}
	length := (*buf)[0]
	if (length & 0x80) != 0 {
		length &= 0x7F
		if len(*buf) < int(length+1) {
			log.WithFields(log.Fields{"data_length": len(*buf), "length": int(length + 1)}).Error("length of data is smaller then encoded length")
			return 0, fmt.Errorf(`input slice has small size`)
		}
		ret = int64(binary.BigEndian.Uint64(append(make([]byte, 8-length), (*buf)[1:length+1]...)))
	} else {
		ret = int64(length)
		length = 0
	}
	*buf = (*buf)[length+1:]
	return
}

func DecodeLengthBuf(buf *bytes.Buffer) (int, error) {
	if buf.Len() == 0 {
		return 0, nil
	}

	length, err := buf.ReadByte()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("cannot read bytes from buffer")
		return 0, err
	}

	if (length & 0x80) == 0 {
		return int(length), nil
	}

	length &= 0x7F
	if buf.Len() < int(length) {
		log.WithFields(log.Fields{"data_length": buf.Len(), "length": int(length), "type": consts.UnmarshallingError}).Error("length of data is smaller then encoded length")
		return 0, fmt.Errorf(`input slice has small size`)
	}

	n := int(binary.BigEndian.Uint64(append(make([]byte, 8-length), buf.Next(int(length))...)))
	if n < 0 {
		return 0, fmt.Errorf(`input slice has negative size`)
	}

	return n, nil
}

func DecodeBytesBuf(buf *bytes.Buffer) ([]byte, error) {
	n, err := DecodeLengthBuf(buf)
	if err != nil {
		return nil, err
	}
	if buf.Len() < n {
		return nil, ErrSliceSize
	}
	return buf.Next(n), nil
}

// BinMarshal converts v parameter to []byte slice.
func BinMarshal(out *[]byte, v any) (*[]byte, error) {
	var err error

	t := reflect.ValueOf(v)
	if *out == nil {
		*out = make([]byte, 0, 2048)
	}

	switch t.Kind() {
	case reflect.Uint8, reflect.Int8:
		*out = append(*out, uint8(t.Uint()))
	case reflect.Uint32:
		tmp := make([]byte, 4)
		binary.BigEndian.PutUint32(tmp, uint32(t.Uint()))
		*out = append(*out, tmp...)
	case reflect.Int32:
		if uint32(t.Int()) < 128 {
			*out = append(*out, uint8(t.Int()))
		} else {
			var i uint8
			tmp := make([]byte, 4)
			binary.BigEndian.PutUint32(tmp, uint32(t.Int()))
			for ; i < 4; i++ {
				if tmp[i] != uint8(0) {
					break
				}
			}
			*out = append(*out, 128+4-i)
			*out = append(*out, tmp[i:]...)
		}
	case reflect.Float64:
		bin := float2Bytes(t.Float())
		*out = append(*out, bin...)
	case reflect.Int64:
		EncodeLenInt64(out, t.Int())
	case reflect.Uint64:
		tmp := make([]byte, 8)
		binary.BigEndian.PutUint64(tmp, t.Uint())
		*out = append(*out, tmp...)
	case reflect.String:
		*out = append(append(*out, EncodeLength(int64(t.Len()))...), []byte(t.String())...)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if out, err = BinMarshal(out, t.Field(i).Interface()); err != nil {
				return out, err
			}
		}
	case reflect.Slice:
		*out = append(append(*out, EncodeLength(int64(t.Len()))...), t.Bytes()...)
	case reflect.Ptr:
		if out, err = BinMarshal(out, t.Elem().Interface()); err != nil {
			return out, err
		}
	default:
		return out, fmt.Errorf(`unsupported type of BinMarshal`)
	}
	return out, nil
}

func BinUnmarshalBuff(buf *bytes.Buffer, v any) error {
	t := reflect.ValueOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if buf.Len() == 0 {
		log.WithFields(log.Fields{"type": consts.UnmarshallingError, "error": "input slice is empty"}).Error("input slice is empty")
		return fmt.Errorf(`input slice is empty`)
	}
	switch t.Kind() {
	case reflect.Uint8, reflect.Int8:
		val, err := buf.ReadByte()
		if err != nil {
			return err
		}
		t.SetUint(uint64(val))

	case reflect.Uint32:
		t.SetUint(uint64(binary.BigEndian.Uint32(buf.Next(4))))

	case reflect.Int32:
		val, err := buf.ReadByte()
		if err != nil {
			log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("reading bytes from buffer")
			return err
		}
		if val < 128 {
			t.SetInt(int64(val))
		} else {
			var i uint8
			size := val - 128
			tmp := make([]byte, 4)
			if buf.Len() <= int(size) || size > 4 {
				log.WithFields(log.Fields{"type": consts.UnmarshallingError, "data_length": buf.Len(), "length": int(size)}).Error("bin unmarshalling int32")
				return fmt.Errorf(`wrong input data`)
			}
			for ; i < size; i++ {
				byteVal, err := buf.ReadByte()
				if err != nil {
					log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("reading bytes from buffer")
					return err
				}
				tmp[4-size+i] = byteVal
			}
			t.SetInt(int64(binary.BigEndian.Uint32(tmp)))
		}
	case reflect.Float64:
		t.SetFloat(bytes2Float(buf.Next(8)))

	case reflect.Int64:
		val, err := DecodeLenInt64Buf(buf)
		if err != nil {
			return err
		}
		t.SetInt(val)

	case reflect.Uint64:
		t.SetUint(binary.BigEndian.Uint64(buf.Next(8)))

	case reflect.String:
		val, err := DecodeLengthBuf(buf)
		if err != nil {
			return err
		}
		if buf.Len() < val {
			log.WithFields(log.Fields{"type": consts.UnmarshallingError, "data_length": buf.Len(), "length": val}).Error("bin unmarshalling string")
			return fmt.Errorf(`input slice is short`)
		}
		t.SetString(string(buf.Next(val)))

	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if err := BinUnmarshalBuff(buf, t.Field(i).Addr().Interface()); err != nil {
				return err
			}
		}
	case reflect.Slice:
		val, err := DecodeLengthBuf(buf)
		if err != nil {
			return err
		}
		if buf.Len() < val {
			log.WithFields(log.Fields{"type": consts.UnmarshallingError, "data_length": buf.Len(), "length": val}).Error("bin unmarshalling slice")
			return fmt.Errorf(`input slice is short`)
		}
		t.SetBytes(buf.Next(val))

	default:
		log.WithFields(log.Fields{"type": consts.UnmarshallingError, "value_type": t.Kind()}).Error("BinUnmrashal unsupported type")
		return fmt.Errorf(`unsupported type of BinUnmarshal %v`, t.Kind())
	}
	return nil

}

// BinUnmarshal converts []byte slice which has been made with BinMarshal to v
func BinUnmarshal(out *[]byte, v any) error {
	t := reflect.ValueOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if len(*out) == 0 {
		return fmt.Errorf(`input slice is empty`)
	}
	switch t.Kind() {
	case reflect.Uint8, reflect.Int8:
		val := uint64((*out)[0])
		t.SetUint(val)
		*out = (*out)[1:]
	case reflect.Uint32:
		t.SetUint(uint64(binary.BigEndian.Uint32((*out)[:4])))
		*out = (*out)[4:]
	case reflect.Int32:
		val := (*out)[0]
		if val < 128 {
			t.SetInt(int64(val))
			*out = (*out)[1:]
		} else {
			var i uint8
			size := val - 128
			tmp := make([]byte, 4)
			if len(*out) <= int(size) || size > 4 {
				return fmt.Errorf(`wrong input data`)
			}
			for ; i < size; i++ {
				tmp[4-size+i] = (*out)[i+1]
			}
			t.SetInt(int64(binary.BigEndian.Uint32(tmp)))
			*out = (*out)[size+1:]
		}
	case reflect.Float64:
		t.SetFloat(bytes2Float((*out)[:8]))
		*out = (*out)[8:]
	case reflect.Int64:
		val, err := DecodeLenInt64(out)
		if err != nil {
			return err
		}
		t.SetInt(val)
	case reflect.Uint64:
		t.SetUint(binary.BigEndian.Uint64((*out)[:8]))
		*out = (*out)[8:]
	case reflect.String:
		val, err := DecodeLength(out)
		if err != nil {
			return err
		}
		if len(*out) < int(val) {
			log.WithFields(log.Fields{"type": consts.UnmarshallingError, "data_length": len(*out), "length": int(val)}).Error("input slice is short")
			return fmt.Errorf(`input slice is short`)
		}
		t.SetString(string((*out)[:val]))
		*out = (*out)[val:]
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if err := BinUnmarshal(out, t.Field(i).Addr().Interface()); err != nil {
				return err
			}
		}
	case reflect.Slice:
		val, err := DecodeLength(out)
		if err != nil {
			return err
		}
		if len(*out) < int(val) {
			return fmt.Errorf(`input slice is short`)
		}
		t.SetBytes((*out)[:val])
		*out = (*out)[val:]
	default:
		log.WithFields(log.Fields{"type": consts.UnmarshallingError, "value_type": t.Kind()}).Error("BinUnmrashal unsupported type")
		return fmt.Errorf(`unsupported type of BinUnmarshal %v`, t.Kind())
	}
	return nil
}

// Sanitize deletes unaccessable characters from input string
func Sanitize(name string, available string) string {
	out := make([]rune, 0, len(name))
	for _, ch := range name {
		if ch > 127 || (ch >= '0' && ch <= '9') || ch == '_' || (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') || strings.IndexRune(available, ch) >= 0 {
			out = append(out, ch)
		}
	}
	return string(out)
}

// SanitizeScript deletes unaccessable characters from input string
func SanitizeScript(input string) string {
	return strings.Replace(strings.Replace(input, `<script`, `&lt;script`, -1), `script>`, `script&gt;`, -1)
}

// SanitizeName deletes unaccessable characters from name string
func SanitizeName(input string) string {
	return Sanitize(input, `- `)
}

// SanitizeNumber deletes unaccessable characters from number or name string
func SanitizeNumber(input string) string {
	return Sanitize(input, `+.- `)
}

func EscapeSQL(name string) string {
	return strings.Replace(strings.Replace(strings.Replace(name, `"`, `""`, -1),
		`;`, ``, -1), `'`, `''`, -1)
}

// EscapeName deletes unaccessable characters for input name(s)
func EscapeName(name string) string {
	out := make([]byte, 1, len(name)+2)
	out[0] = '"'
	available := `() ,`
	for _, ch := range []byte(name) {
		if (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' || (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') || strings.IndexByte(available, ch) >= 0 {
			out = append(out, ch)
		}
	}
	if strings.IndexAny(string(out), available) >= 0 {
		return string(out[1:])
	}
	return string(append(out, '"'))
}

// Float2Bytes converts float64 to []byte
func float2Bytes(float float64) []byte {
	ret := make([]byte, 8)
	binary.LittleEndian.PutUint64(ret, math.Float64bits(float))
	return ret
}

// Bytes2Float converts []byte to float64
func bytes2Float(bytes []byte) float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(bytes))
}

// UInt32ToStr converts uint32 to string
func UInt32ToStr(num uint32) string {
	return strconv.FormatInt(int64(num), 10)
}

// Int64ToStr converts int64 to string
func Int64ToStr(num int64) string {
	return strconv.FormatInt(num, 10)
}

// Int64ToByte converts int64 to []byte
func Int64ToByte(num int64) []byte {
	return []byte(strconv.FormatInt(num, 10))
}

// IntToStr converts integer to string
func IntToStr(num int) string {
	return strconv.Itoa(num)
}

// DecToBin converts interface to []byte
func DecToBin(v any, sizeBytes int64) []byte {
	var dec int64
	switch v.(type) {
	case int:
		dec = int64(v.(int))
	case int64:
		dec = v.(int64)
	case uint64:
		dec = int64(v.(uint64))
	case string:
		dec = StrToInt64(v.(string))
	}
	Hex := fmt.Sprintf("%0"+Int64ToStr(sizeBytes*2)+"x", dec)
	return HexToBin([]byte(Hex))
}

// BinToHex converts interface to hex []byte
func BinToHex(v any) []byte {
	var bin []byte
	switch v.(type) {
	case []byte:
		bin = v.([]byte)
	case int64:
		bin = Int64ToByte(v.(int64))
	case string:
		bin = []byte(v.(string))
	}
	return []byte(fmt.Sprintf("%x", bin))
}

// HexToBin converts hex interface to binary []byte
func HexToBin(ihexdata any) []byte {
	var hexdata string
	switch ihexdata.(type) {
	case []byte:
		hexdata = string(ihexdata.([]byte))
	case int64:
		hexdata = Int64ToStr(ihexdata.(int64))
	case string:
		hexdata = ihexdata.(string)
	}
	var str []byte
	str, err := hex.DecodeString(hexdata)
	if err != nil {
		log.WithFields(log.Fields{"data": hexdata, "error": err, "type": consts.ConversionError}).Error("decoding string to hex")
		log.Printf("HexToBin error: %s", err)
	}
	return str
}

// BinToDec converts input binary []byte to int64
func BinToDec(bin []byte) int64 {
	var a uint64
	l := len(bin)
	for i, b := range bin {
		shift := uint64((l - i - 1) * 8)
		a |= uint64(b) << shift
	}
	return int64(a)
}

// BinToDecBytesShift converts the input binary []byte to int64 and shifts the input bin
func BinToDecBytesShift(bin *[]byte, num int64) int64 {
	return BinToDec(BytesShift(bin, num))
}

// BytesShift returns the index bytes of the input []byte and shift str pointer
func BytesShift(str *[]byte, index int64) (ret []byte) {
	if int64(len(*str)) < index || index == 0 {
		*str = (*str)[:0]
		return []byte{}
	}
	ret, *str = (*str)[:index], (*str)[index:]
	return
}

// InterfaceToStr converts the interfaces to the string
func InterfaceToStr(v any) (string, error) {
	var str string
	if v == nil {
		return ``, nil
	}
	switch v.(type) {
	case int:
		str = IntToStr(v.(int))
	case float64:
		str = Float64ToStr(v.(float64))
	case int64:
		str = Int64ToStr(v.(int64))
	case string:
		str = v.(string)
	case []byte:
		str = string(v.([]byte))
	default:
		if reflect.TypeOf(v).String() == `map[string]interface {}` ||
			reflect.TypeOf(v).String() == `*types.Map` {
			if out, err := json.Marshal(v); err != nil {
				log.WithFields(log.Fields{"error": err, "type": consts.JSONMarshallError}).Error("marshalling map for jsonb")
				return ``, err
			} else {
				str = string(out)
			}
		} else if reflect.TypeOf(v).String() == `decimal.Decimal` {
			str = v.(decimal.Decimal).String()
		}
	}
	return str, nil
}

// InterfaceSliceToStr converts the slice of interfaces to the slice of strings
func InterfaceSliceToStr(i []any) (strs []string, err error) {
	var val string
	for _, v := range i {
		val, err = InterfaceToStr(v)
		if err != nil {
			return
		}
		strs = append(strs, val)
	}
	return
}

// InterfaceToFloat64 converts the interfaces to the float64
func InterfaceToFloat64(i any) float64 {
	var result float64
	switch i.(type) {
	case int:
		result = float64(i.(int))
	case float64:
		result = i.(float64)
	case int64:
		result = float64(i.(int64))
	case string:
		result = StrToFloat64(i.(string))
	case []byte:
		result = BytesToFloat64(i.([]byte))
	}
	return result
}

// BytesShiftReverse gets []byte from the end of the input and cut the input pointer to []byte
func BytesShiftReverse(str *[]byte, v any) []byte {
	var index int64
	switch v.(type) {
	case int:
		index = int64(v.(int))
	case int64:
		index = v.(int64)
	}

	var substr []byte
	slen := int64(len(*str))
	if slen < index {
		index = slen
	}
	substr = (*str)[slen-index:]
	*str = (*str)[:slen-index]
	return substr
}

// StrToInt64 converts string to int64
func StrToInt64(s string) int64 {
	ret, _ := strconv.ParseInt(s, 10, 64)
	return ret
}

// BytesToInt64 converts []bytes to int64
func BytesToInt64(s []byte) int64 {
	ret, _ := strconv.ParseInt(string(s), 10, 64)
	return ret
}

// StrToUint64 converts string to the unsinged int64
func StrToUint64(s string) uint64 {
	ret, _ := strconv.ParseUint(s, 10, 64)
	return ret
}

// StrToInt converts string to integer
func StrToInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

// Float64ToStr converts float64 to string
func Float64ToStr(f float64) string {
	return strconv.FormatFloat(f, 'f', 13, 64)
}

// StrToFloat64 converts string to float64
func StrToFloat64(s string) float64 {
	Float64, _ := strconv.ParseFloat(s, 64)
	return Float64
}

// BytesToFloat64 converts []byte to float64
func BytesToFloat64(s []byte) float64 {
	Float64, _ := strconv.ParseFloat(string(s), 64)
	return Float64
}

// BytesToInt converts []byte to integer
func BytesToInt(s []byte) int {
	i, _ := strconv.Atoi(string(s))
	return i
}

// StrToMoney rounds money string to float64
func StrToMoney(str string) float64 {
	ind := strings.Index(str, ".")
	var newStr string
	if ind != -1 {
		end := 2
		if len(str[ind+1:]) > 1 {
			end = 3
		}
		newStr = str[:ind] + "." + str[ind+1:ind+end]
	} else {
		newStr = str
	}
	return StrToFloat64(newStr)
}

// EncodeLengthPlusData encoding interface into []byte
func EncodeLengthPlusData(idata any) []byte {
	var data []byte
	switch idata.(type) {
	case int64:
		data = Int64ToByte(idata.(int64))
	case string:
		data = []byte(idata.(string))
	case []byte:
		data = idata.([]byte)
	}
	//log.Debug("data: %x", data)
	//log.Debug("len data: %d", len(data))
	return append(EncodeLength(int64(len(data))), data...)
}

// FormatMoney converts minimal unit to legibility unit. For example, value * 10 ^ -digit
func FormatMoney(exp string, digit int32) (string, error) {
	if len(exp) == 0 {
		return `0`, nil
	}
	if strings.IndexByte(exp, '.') >= 0 {
		return `0`, fmt.Errorf(`wrong money format %s`, exp)
	}
	if digit < 0 {
		return `0`, fmt.Errorf(`digit must be positive`)
	}
	if len(exp) > consts.MoneyLength {
		return `0`, fmt.Errorf(`too long money`)
	}
	retDec, err := decimal.NewFromString(exp)
	if err != nil {
		return `0`, err
	}
	return retDec.Shift(-digit).String(), nil
}

// EscapeForJSON replaces quote to slash and quote
func EscapeForJSON(data string) string {
	return strings.Replace(data, `"`, `\"`, -1)
}

// ValidateEmail validates email
func ValidateEmail(email string) bool {
	Re := regexp.MustCompile(`^(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
	return Re.MatchString(email)
}

// ParseName gets a state identifier and the name of the contract or table
// from the full name like @[id]name
func ParseName(in string) (id int64, name string) {
	re := regexp.MustCompile(`(?is)^@(\d+)(\w[_\w\d]*)$`)
	ret := re.FindStringSubmatch(in)
	if len(ret) == 3 {
		id = StrToInt64(ret[1])
		name = ret[2]
	}
	return
}

func ParseTable(tblname string, defaultEcosystem int64) string {
	ecosystem, name := ParseName(tblname)
	if ecosystem == 0 {
		if FirstEcosystemTables[tblname] {
			ecosystem = 1
		} else {
			ecosystem = defaultEcosystem
		}
		name = tblname
	}
	return strings.ToLower(fmt.Sprintf(`%d_%s`, ecosystem, Sanitize(name, ``)))
}

func SubNodeParseTable(tblname string, defaultEcosystem int64) string {
	ecosystem, name := ParseName(tblname)
	if ecosystem == 0 {
		if FirstEcosystemTables[tblname] {
			ecosystem = 1
		} else {
			ecosystem = defaultEcosystem
		}
		name = tblname
	}
	//return strings.ToLower(fmt.Sprintf(`%d_%s`, ecosystem, Sanitize(name, ``)))
	return strings.ToLower(fmt.Sprintf(`%s`, Sanitize(name, ``)))
}

// SliceReverse reverses the slice of int64
func SliceReverse(s []int64) []int64 {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

// SortMap sorts map to the slice of maps
func SortMap(m map[int64]string) []map[int64]string {
	var keys []int
	for k := range m {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	var result []map[int64]string
	for _, k := range keys {
		result = append(result, map[int64]string{int64(k): m[int64(k)]})
	}
	return result
}

// RSortMap sorts map to the reversed slice of maps
func RSortMap(m map[int64]string) []map[int64]string {

	var keys []int
	for k := range m {
		keys = append(keys, int(k))
	}
	sort.Sort(sort.Reverse(sort.IntSlice(keys)))
	var result []map[int64]string
	for _, k := range keys {
		result = append(result, map[int64]string{int64(k): m[int64(k)]})
	}
	return result
}

// InSliceString searches the string in the slice of strings
func InSliceString(search string, slice []string) bool {
	for _, v := range slice {
		if v == search {
			return true
		}
	}
	return false
}

// StripTags replaces < and > to &lt; and &gt;
func StripTags(value string) string {
	return strings.Replace(strings.Replace(value, `<`, `&lt;`, -1), `>`, `&gt;`, -1)
}

// IsLatin checks if the specified string contains only latin character, digits and '-', '_'.
func IsLatin(name string) bool {
	for _, ch := range []byte(name) {
		if !((ch >= '0' && ch <= '9') || ch == '_' || ch == '-' || (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z')) {
			return false
		}
	}
	return true
}

// Escape deletes unaccessable characters
func Escape(data string) string {
	out := make([]rune, 0, len(data))
	available := `_ ,=!-'()"?*$#{}<>: `
	for _, ch := range []rune(data) {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') || strings.IndexByte(available, byte(ch)) >= 0 ||
			unicode.IsLetter(ch) || ch >= 128 {
			out = append(out, ch)
		}
	}
	return string(out)
}

// FieldToBytes returns the value of n-th field of v as []byte
func FieldToBytes(v any, num int) []byte {
	t := reflect.ValueOf(v)
	ret := make([]byte, 0, 2048)
	if t.Kind() == reflect.Struct && num < t.NumField() {
		field := t.Field(num)
		switch field.Kind() {
		case reflect.Uint8, reflect.Uint32, reflect.Uint64:
			ret = append(ret, []byte(fmt.Sprintf("%d", field.Uint()))...)
		case reflect.Int8, reflect.Int32, reflect.Int64:
			ret = append(ret, []byte(fmt.Sprintf("%d", field.Int()))...)
		case reflect.Float64:
			ret = append(ret, []byte(fmt.Sprintf("%f", field.Float()))...)
		case reflect.String:
			ret = append(ret, []byte(field.String())...)
		case reflect.Slice:
			ret = append(ret, field.Bytes()...)
			//		case reflect.Ptr:
			//		case reflect.Struct:
			//		default:
		}
	}
	return ret
}

// NumString insert spaces between each three digits. 7123456 => 7 123 456
func NumString(in string) string {
	if strings.IndexByte(in, '.') >= 0 {
		lr := strings.Split(in, `.`)
		return NumString(lr[0]) + `.` + lr[1]
	}
	buf := []byte(in)
	out := make([]byte, len(in)+4)
	for len(buf) > 3 {
		out = append(append([]byte(` `), buf[len(buf)-3:]...), out...)
		buf = buf[:len(buf)-3]
	}
	return string(append(buf, out...))
}

func Round(num float64) int64 {
	//log.Debug("num", num)
	//num += ROUND_FIX
	//	return int(StrToFloat64(Float64ToStr(num)) + math.Copysign(0.5, num))
	//log.Debug("num", num)
	return int64(num + math.Copysign(0.5, num))
}

// RoundWithPrecision rounds float64 value
func RoundWithPrecision(num float64, precision int) float64 {
	num += consts.RoundFix
	output := math.Pow(10, float64(precision))
	return float64(Round(num*output)) / output
}

// RoundWithoutPrecision is round float64 without precision
func RoundWithoutPrecision(num float64) int64 {
	//log.Debug("num", num)
	//num += ROUND_FIX
	//	return int(StrToFloat64(Float64ToStr(num)) + math.Copysign(0.5, num))
	//log.Debug("num", num)
	return int64(num + math.Copysign(0.5, num))
}

// ValueToInt converts interface (string or int64) to int64
func ValueToInt(v any) (ret int64, err error) {
	switch val := v.(type) {
	case float64:
		ret = int64(val)
	case int64:
		ret = val
	case string:
		if len(val) == 0 {
			return 0, nil
		}
		ret, err = strconv.ParseInt(val, 10, 64)
		if err != nil {
			errText := err.Error()
			if strings.Contains(errText, `:`) {
				errText = errText[strings.LastIndexByte(errText, ':'):]
			} else {
				errText = ``
			}
			err = fmt.Errorf(`%s is not a valid integer %s`, val, errText)
		}
	case decimal.Decimal:
		ret = val.IntPart()
	case json.Number:
		ret, err = val.Int64()
	default:
		if v == nil {
			return 0, nil
		}
		err = fmt.Errorf(`%v is not a valid integer`, val)
	}
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ConversionError, "error": err,
			"value": fmt.Sprint(v)}).Error("converting value to int")
	}
	return
}

func ValueToDecimal(v any) (ret decimal.Decimal, err error) {
	switch val := v.(type) {
	case float64:
		ret = decimal.NewFromFloat(val).Floor()
	case string:
		ret, err = decimal.NewFromString(val)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "value": val}).Error("converting value from string to decimal")
		} else {
			ret = ret.Floor()
		}
	case int64:
		ret = decimal.New(val, 0)
	default:
		ret = val.(decimal.Decimal)
	}
	return
}

func Int64ToDateStr(date int64, format string) string {
	t := time.Unix(date, 0)
	return t.Format(format)
}

func Int64Toint(dat int64) (int, error) {
	str := strconv.FormatInt(dat, 10)
	return strconv.Atoi(str)
}

func MarshalJson(v any) string {
	buff, err := json.Marshal(v)
	if err != nil {
		log.WithFields(log.Fields{"v": v, "error": err}).Error("marshalJson error")
	}
	return string(buff)
}
