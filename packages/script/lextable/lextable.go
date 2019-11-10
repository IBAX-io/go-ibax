/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// The program creates packages/script/lex_table.go files.

// Action is a map of actions
type Action map[string][]string

// States is a map of states
type States map[string]Action

const (
	// AlphaSize is the length of alphabet
	AlphaSize = 34
)

/* Здесь мы определяем алфавит, с которым будет работать наш язык и описываем конечный автомат, который
   переходит из одного состояния в другое в зависимости от очередного полученного символа.
   Данная программа переводит список состояний в числовой массив и сохраняет его как
   packages/script/lex_table.go
*/

var (
	table [][AlphaSize]uint32
	lexem = map[string]uint32{``: 0, `sys`: 1, `oper`: 2, `number`: 3, `ident`: 4, `newline`: 5, `string`: 6,
		`comment`: 7}
	flags    = map[string]uint32{`next`: 1, `push`: 2, `pop`: 4, `skip`: 8}
	alphabet = []byte{0x01, 0x0a, ' ', '`', '"', ';', '(', ')', '[', ']', '{', '}', '&',
		//           default  n    s    q    Q
		'|', '#', '.', ',', '<', '>', '=', '!', '*', '$', '@', ':',
		'+', '-', '/', '\\', '0', '1', 'a', '_', 128}
	//													r

	// В states мы обозначили за d - все символы, которые не указаны в состоянии
	// n - 0x0a, s - пробел, q - обратные кавычки `, Q - двойные кавычки, r - символы >= 128
	// a - A-Z и a-z, 1 - 1-9
	// В качестве ключей выступаю имена состояний, a в объекте-значении перечислены возможные наборы символов
	// и затем для каждого такого набора идет новое состояние, куда следует сделать переход, далее имя лексемы,
	// если нам нужно вернуться в начальное состояние и третьим параметром идут служебные флаги,
	// которые указывают, что делать с текущим символом.
	// Например, у нас сотояние main и входящий символ /. push говорит запомнить его в отедльном стеке и
	// next - перейти к следующему символу, при этом мы меняе состояние на solidus.
	// Берем следующий символ и смотрим на состоние solidus
	// Если у нас / или * - то мы переходим в состояние комментарий, так они начинаются с // или /*.
	// При этом видно, что для каждого комментария разные последующие состояния, так как заканчиваются
	// они разными символами.  А если у нас следующий символ не / и не *, то мы все что у нас положено
	// в стэк (/) записываем как лексему с типом oper, очищаем стэк и возвращаемся в состояние main.
	states = `{
	"main": {
			"n;": ["main", "newline", "next"],
			"()#[],{}:": ["main", "sys", "next"],
			"s": ["main", "", "next"],
			"q": ["string", "", "push next"],
			"Q": ["dstring", "", "push next"],
			"&": ["and", "", "push next"],
			"|": ["or", "", "push next"],
			"=": ["eq", "", "push next"],
			"/": ["solidus", "", "push next"],
			"<>!": ["oneq", "", "push next"],
			"*+-": ["main", "oper", "next"],
			"01": ["number", "", "push next"],
			"a_r": ["ident", "", "push next"],
			"@$": ["mustident", "", "push next"],
			".": ["dot", "", "push next"],
			"d": ["error", "", ""]
		},
	"string": {
			"q": ["main", "string", "pop next"],
			"d": ["string", "", "next"]
		},
	"dstring": {
			"Q": ["main", "string", "pop next"],
			"\\": ["dslash", "", "skip"],			
			"d": ["dstring", "", "next"]
		},
	"dslash": {
		"d": ["dstring", "", "next"]
	},		
	"dot": {
		".": ["ddot", "", "next"],
		"01": ["number", "", "next"],
		"d": ["main", "sys", "pop"]
	},
	"ddot": {
		".": ["main", "ident", "pop next"],
		"d": ["error", "", ""]
	},
	"and": {
			"&": ["main", "oper", "pop next"],
			"d": ["error", "", ""]
		},
	"or": {
			"|": ["main", "oper", "pop next"],
			"d": ["error", "", ""]
		},
	"eq": {
			"=": ["main", "oper", "pop next"],
			"d": ["main", "sys", "pop"]
		},
	"solidus": {
			"/": ["comline", "", "pop next"],
			"*": ["comment", "", "next"],
			"d": ["main", "oper", "pop"]
		},
	"oneq": {
			"=": ["main", "oper", "pop next"],
			"d": ["main", "oper", "pop"]
		},
	"number": {
			"01.": ["number", "", "next"],
			"a_r": ["error", "", ""],
			"d": ["main", "number", "pop"]
		},
	"ident": {
			"01a_r": ["ident", "", "next"],
			"d": ["main", "ident", "pop"]
		},
	"mustident": {
		"01a_r": ["ident", "", "next"],
		"d": ["error", "", ""]
	},
	"comment": {
			"*": ["comstop", "", "next"],
			"d": ["comment", "", "next"]
		},
	"comstop": {
			"/": ["main", "comment", "pop next"],
			"d": ["comment", "", "next"]
		},
	"comline": {
			"n": ["main", "", ""],
			"d": ["comline", "", "next"]
		}
}`
)

func main() {
	var alpha [129]byte
	for ind, ch := range alphabet {
		i := byte(ind)
		switch ch {
		case ' ':
			alpha[0x09] = i
			alpha[0x0d] = i
			alpha[' '] = i
		case '1':
			for k := '1'; k <= '9'; k++ {
				alpha[k] = i
			}
		case 'a':
			for k := 'A'; k <= 'Z'; k++ {
				alpha[k] = i
			}
			for k := 'a'; k <= 'z'; k++ {
				alpha[k] = i
			}
		case 128:
			alpha[128] = i
		default:
			alpha[ch] = i
		}
	}
	out := `package script
	// This file was generated with lextable.go
	
var (
		alphabet = []byte{`
	for i, ch := range alpha {
		out += fmt.Sprintf(`%d,`, ch)
		if i > 0 && i%24 == 0 {
			out += "\r\n\t\t\t"
		}
	}
	out += "\r\n\t\t}\r\n"

	var (
		data States
	)
	state2int := map[string]uint{`main`: 0}
	if err := json.Unmarshal([]byte(states), &data); err == nil {
		for key := range data {
			if key != `main` {
				state2int[key] = uint(len(state2int))
			}
		}
		table = make([][AlphaSize]uint32, len(state2int))
		for key, istate := range data {
			curstate := state2int[key]
			for i := range table[curstate] {
				table[curstate][i] = 0xFE0000
			}

			for skey, sval := range istate {
				var val uint32
				if sval[0] == `error` {
					val = 0xff0000
				} else {
					val = uint32(state2int[sval[0]] << 16) // new state
				}
				val |= uint32(lexem[sval[1]] << 8) // lexem
				cmds := strings.Split(sval[2], ` `)
				var flag uint32
				for _, icmd := range cmds {
					flag |= flags[icmd]
				}
				val |= flag
				for _, ch := range []byte(skey) {
					var ind int
					switch ch {
					case 'd':
						ind = 0
					case 'n':
						ind = 1
					case 's':
						ind = 2
					case 'q':
						ind = 3
					case 'Q':
						ind = 4
					case 'r':
						ind = AlphaSize - 1
					default:
						for k, ach := range alphabet {
							if ach == ch {
								ind = k
								break
							}
						}
					}
					table[curstate][ind] = val
					if ind == 0 { // default value
						for i := range table[curstate] {
							if table[curstate][i] == 0xFE0000 {
								table[curstate][i] = val
							}
						}
					}
				}
			}
		}
		out += "\t\tlexTable = [][" + fmt.Sprint(AlphaSize) + "]uint32{\r\n"
		for _, line := range table {
			out += "\t\t\t{"
			for _, ival := range line {
				out += fmt.Sprintf(" 0x%x,", ival)
			}
			out += "\r\n\t\t\t},\r\n"
		}
		out += "\t\t\t}\r\n)\r\n"
		err = os.WriteFile("../lex_table.go", []byte(out), 0644)
		if err != nil {
			fmt.Println(err.Error())
		}
	} else {
		fmt.Println(err.Error())
	}
