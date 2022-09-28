/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

// The lexical analysis of the incoming program is implemented in this file. It is the first phase of compilation
// where the incoming text is divided into a sequence of lexemes.

const (
	// Here are all the created lexemes
	lexUnknown = iota
	lexSys     // a system lexeme is different bracket, =, comma and so on.
	lexOper    // Operator is +, -, *, /
	lexNumber  // Number
	lexIdent   // Identifier
	lexNewLine // Line translation
	lexString  // String
	lexComment // Comment
	lexKeyword // Key word
	lexType    // Name of the type
	lexExtend  // Referring to an external variable or function - $myname

	lexError = 0xff
	// flags of lexical states
	lexfNext = 1
	lexfPush = 2
	lexfPop  = 4
	lexfSkip = 8

	// Constants for system lexemes
	isLPar   = 0x2801 // (
	isRPar   = 0x2901 // )
	isComma  = 0x2c01 // ,
	isDot    = 0x2e01 // .
	isColon  = 0x3a01 // :
	isEq     = 0x3d01 // =
	isLCurly = 0x7b01 // {
	isRCurly = 0x7d01 // }
	isLBrack = 0x5b01 // [
	isRBrack = 0x5d01 // ]

	// Constants for operations
	isNot      = 0x0021 // !
	isAsterisk = 0x002a // *
	isPlus     = 0x002b // +
	isMinus    = 0x002d // -
	isSign     = 0x012d // - unary
	isSolidus  = 0x002f // /
	isLess     = 0x003c // <
	isGreat    = 0x003e // >
	isNotEq    = 0x213d // !=
	isAnd      = 0x2626 // &&
	isLessEq   = 0x3c3d // <=
	isEqEq     = 0x3d3d // ==
	isGrEq     = 0x3e3d // >=
	isOr       = 0x7c7c // ||

)

const (
	// The list of keyword identifiers
	// Constants for keywords
	//	keyUnknown = iota
	keyContract = iota + 1
	keyFunc
	keyReturn
	keyIf
	keyElif
	keyElse
	keyWhile
	keyTrue
	keyFalse
	keyVar
	keyTX
	keySettings
	keyBreak
	keyContinue
	keyWarning
	keyInfo
	keyNil
	keyAction
	keyCond
	keyTail
	keyError
)

const (
	msgWarning = `warning`
	msgError   = `error`
	msgInfo    = `info`
)

const (
	DtBool uint32 = iota + 1
	DtBytes
	DtInt
	DtAddress
	DtArray
	DtMap
	DtMoney
	DtFloat
	DtString
	DtFile
)

type typeInfo struct {
	Original uint32
	Type     reflect.Type
}

var (
	// The list of keywords
	keywords = map[string]uint32{
		`contract`:   keyContract,
		`func`:       keyFunc,
		`return`:     keyReturn,
		`if`:         keyIf,
		`elif`:       keyElif,
		`else`:       keyElse,
		msgError:     keyError,
		msgWarning:   keyWarning,
		msgInfo:      keyInfo,
		`while`:      keyWhile,
		`data`:       keyTX,
		`settings`:   keySettings,
		`nil`:        keyNil,
		`action`:     keyAction,
		`conditions`: keyCond,
		`true`:       keyTrue,
		`false`:      keyFalse,
		`break`:      keyBreak,
		`continue`:   keyContinue,
		`var`:        keyVar,
		`...`:        keyTail}

	// list of available types
	// The list of types which save the corresponding 'reflect' type
	typesMap = map[string]typeInfo{
		`bool`:    {Original: DtBool, Type: reflect.TypeOf(true)},
		`bytes`:   {Original: DtBytes, Type: reflect.TypeOf([]byte{})},
		`int`:     {Original: DtInt, Type: reflect.TypeOf(int64(0))},
		`address`: {Original: DtAddress, Type: reflect.TypeOf(int64(0))},
		`array`:   {Original: DtArray, Type: reflect.TypeOf([]any{})},
		`map`:     {Original: DtMap, Type: reflect.TypeOf(&types.Map{})},
		`money`:   {Original: DtMoney, Type: reflect.TypeOf(decimal.Zero)},
		`float`:   {Original: DtFloat, Type: reflect.TypeOf(0.0)},
		`string`:  {Original: DtString, Type: reflect.TypeOf(``)},
		`file`:    {Original: DtFile, Type: reflect.TypeOf(&types.Map{})},
	}
)

func GetFieldDefaultValue(fieldType uint32) any {
	switch fieldType {
	case DtBool:
		return false
	case DtFloat:
		return float64(0)
	case DtInt, DtAddress:
		return int64(0)
	case DtMoney:
		return decimal.New(0, consts.MoneyDigits)
	case DtString:
		return ""
	case DtBytes:
		return []byte{}
	case DtArray:
		return []any{}
	case DtMap:
		return types.NewMap()
	case DtFile:
		return types.NewFile()
	}
	return nil
}

// Lexeme contains information about language item
type Lexeme struct {
	Type   uint32 // Type of the lexeme
	Ext    uint32
	Value  any    // Value of lexeme
	Line   uint16 // Line of the lexeme
	Column uint32 // Position inside the line
}

func NewLexeme(t uint32, ext uint32, value any, line uint16, column uint32) *Lexeme {
	return &Lexeme{Type: t, Ext: ext, Value: value, Line: line, Column: column}
}

// GetLogger returns logger
func (l *Lexeme) GetLogger() *log.Entry {
	return log.WithFields(log.Fields{"lex_type": l.Type, "lex_line": l.Line, "lex_column": l.Column})
}

type ifBuf struct {
	count int
	pair  int
	stop  bool
}

// Lexemes is a slice of lexemes
type Lexemes []*Lexeme

// The lexical analysis is based on the finite machine which is described in the file
// tools/lextable/lextable.go. lextable.go generates a representation of a finite machine as an array
// and records it in the file lex_table.go. In fact, the lexTable array is a set of states and
// depending on the next sign, the machine goes into a new state.
// lexParser parsers the input language source code
func lexParser(input []rune) (Lexemes, error) {
	var (
		curState                                        uint8
		length, line, off, offline, flags, start, lexID uint32
	)

	lexemes := make(Lexemes, 0, len(input)/4)
	irune := len(alphabet) - 1

	// This function according to the next symbol looks with help of lexTable what new state we will have,
	// whether we got the lexeme and what flags are displayed
	todo := func(r rune) {
		var letter uint8
		if r > 127 {
			letter = alphabet[irune]
		} else {
			letter = alphabet[r]
		}
		val := lexTable[curState][letter]
		curState = uint8(val >> 16)
		lexID = (val >> 8) & 0xff
		flags = val & 0xff
	}
	length = uint32(len(input)) + 1
	line = 1
	skip := false
	ifbuf := make([]ifBuf, 0)
	for off < length {
		// Here we go through the symbols one by one
		if off == length-1 {
			todo(' ')
		} else {
			todo(input[off])
		}
		if curState == lexError {
			return nil, fmt.Errorf(`unknown lexeme '%s' [Ln:%d Col:%d]`,
				string(input[off:off+1]), line, off-offline+1)
		}
		if (flags & lexfSkip) != 0 {
			off++
			skip = true
			continue
		}
		// If machine determined the completed lexeme, we record it in the list of lexemes.
		if lexID > 0 {
			// We do not start a stack for symbols but memorize the displacement when the parse of lexeme began.
			// To get a string of a lexeme we take a substring from the initial displacement to the current one.
			// We immediately write a string as values, a number or a binary representation of operations.
			var ext uint32
			lexOff := off
			if (flags & lexfPop) != 0 {
				lexOff = start
			}
			right := off
			if (flags & lexfNext) != 0 {
				right++
			}
			if len(ifbuf) > 0 && ifbuf[len(ifbuf)-1].stop && lexID != lexNewLine {
				name := string(input[lexOff:right])
				if name != `else` && name != `elif` {
					for i := 0; i < ifbuf[len(ifbuf)-1].count; i++ {
						lexemes = append(lexemes, NewLexeme(lexSys|(uint32('}')<<8), ext, uint32('}'), uint16(line), lexOff-offline+1))
					}
					ifbuf = ifbuf[:len(ifbuf)-1]
				} else {
					ifbuf[len(ifbuf)-1].stop = false
				}
			}
			var value any
			switch lexID {
			case lexNewLine:
				if input[lexOff] == rune(0x0a) {
					line++
					offline = off
				}
			case lexSys:
				ch := uint32(input[lexOff])
				lexID |= ch << 8
				value = ch
				if len(ifbuf) > 0 {
					if ch == '{' {
						ifbuf[len(ifbuf)-1].pair++
					}
					if ch == '}' {
						ifbuf[len(ifbuf)-1].pair--
						if ifbuf[len(ifbuf)-1].pair == 0 {
							ifbuf[len(ifbuf)-1].stop = true
						}
					}
				}
			case lexString, lexComment:
				val := string(input[lexOff+1 : right-1])
				if lexID == lexString && skip {
					skip = false
					val = strings.Replace(strings.Replace(val, `\"`, `"`, -1), `\t`, "\t", -1)
					val = strings.Replace(strings.Replace(val, `\r`, "\r", -1), `\n`, "\n", -1)
				}
				value = val
				for i, ch := range val {
					if ch == 0xa {
						line++
						offline = off + uint32(i) + 1
					}
				}
			case lexOper:
				oper := []byte(string(input[lexOff:right]))
				value = binary.BigEndian.Uint32(append(make([]byte, 4-len(oper)), oper...))
			case lexNumber:
				name := string(input[lexOff:right])
				if strings.ContainsAny(name, `.`) {
					if val, err := strconv.ParseFloat(name, 64); err == nil {
						value = val
					} else {
						log.WithFields(log.Fields{"error": err, "value": name, "lex_line": line, "lex_col": off - offline + 1, "type": consts.ConversionError}).Error("converting lex number to float")
						return nil, fmt.Errorf(`%v %s [Ln:%d Col:%d]`, err, name, line, off-offline+1)
					}
				} else if val, err := strconv.ParseInt(name, 10, 64); err == nil {
					value = val
				} else {
					log.WithFields(log.Fields{"error": err, "value": name, "lex_line": line, "lex_col": off - offline + 1, "type": consts.ConversionError}).Error("converting lex number to int")
					return nil, fmt.Errorf(`%v %s [Ln:%d Col:%d]`, err, name, line, off-offline+1)
				}
			case lexIdent:
				name := string(input[lexOff:right])
				if name[0] == '$' {
					lexID = lexExtend
					value = name[1:]
				} else if keyID, ok := keywords[name]; ok {
					switch keyID {
					case keyIf:
						ifbuf = append(ifbuf, ifBuf{})
						lexID = lexKeyword | (keyID << 8)
						value = keyID
					case keyElif:
						if len(ifbuf) > 0 {
							lexemes = append(lexemes,
								NewLexeme(lexKeyword|(keyElse<<8), ext, uint32(keyElse), uint16(line), lexOff-offline+1),
								NewLexeme(lexSys|('{'<<8), ext, uint32('{'), uint16(line), lexOff-offline+1))
							lexID = lexKeyword | (keyIf << 8)
							value = uint32(keyIf)
							ifbuf[len(ifbuf)-1].count++
						}
					case keyAction, keyCond:
						if len(lexemes) > 0 {
							lexf := *lexemes[len(lexemes)-1]
							if lexf.Type&0xff != lexKeyword || lexf.Value.(uint32) != keyFunc {
								lexemes = append(lexemes, NewLexeme(lexKeyword|(keyFunc<<8), ext, uint32(keyFunc), uint16(line), lexOff-offline+1))
							}
						}
						value = name
					case keyTrue:
						lexID = lexNumber
						value = true
					case keyFalse:
						lexID = lexNumber
						value = false
					case keyNil:
						lexID = lexNumber
						value = nil
					default:
						lexID = lexKeyword | (keyID << 8)
						value = keyID
					}
				} else if tInfo, ok := typesMap[name]; ok {
					lexID = lexType
					value = tInfo.Type
					ext = tInfo.Original
				} else {
					value = name
				}
			}
			if lexID != lexComment {
				lexemes = append(lexemes, NewLexeme(lexID, ext, value, uint16(line), lexOff-offline+1))
			}
		}
		if (flags & lexfPush) != 0 {
			start = off
		}
		if (flags & lexfNext) != 0 {
			off++
		}
	}
	return lexemes, nil
}

func OriginalToString(original uint32) string {
	for key, v := range typesMap {
		if v.Original == original {
			return key
		}
	}
	return ``
}
