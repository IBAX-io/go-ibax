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

/*
Here we define the alphabet with which our language will work and describe the state machine that
passes from one state to another depending on the next received character.
This program converts the list of states into a numeric array and saves it as packages/script/lex_table.go
*/
var (
	table  [][AlphaSize]uint32
	lexeme = map[string]uint32{
		``:        0,
		`sys`:     1,
		`oper`:    2,
		`number`:  3,
		`ident`:   4,
		`newline`: 5,
		`string`:  6,
		`comment`: 7,
	}
	flags = map[string]uint32{
		`next`: 1,
		`push`: 2,
		`pop`:  4,
		`skip`: 8,
	}
	alphabet = []byte{
		0x01, //default
		0x0a, //newline
		' ',  //space
		'`',  //back quotes
		'"',  //double quotes
		';',
		'(',
		')',
		'[',
		']',
		'{',
		'}',
		'&',
		'|',
		'#',
		'.',
		',',
		'<',
		'>',
		'=',
		'!',
		'*',
		'$',
		'@',
		':',
		'+',
		'-',
		'/',
		'\\',
		'0',
		'1',
		'a',
		'_',
		128,
	}
	/*
		In states we have designated for
		d - all characters that are not specified in the state
		n - 0x0a,
		s - space,
		q - back quotes `,
		Q - double quotes,
		r - characters >= 128,
		a - A-Z and a-z,
		1 - 1-9
		State names are used as keys, and possible character sets are listed in the value object
		and then for each such set there is a new state where the transition should be made, then the name of the token,
		if we need to return to the initial state and the service flags are the third parameter,
		which indicate what to do with the current symbol.
		For example, we have the state main and the incoming symbol /. push says to remember it in a separate stack and
		next - go to the next character, while we change the state to solidus.
		Take the next character and look at the state of solidus
		If we have / or * - then we go into the comment state, so they start with // or / *.
		At the same time, you can see that for each comment, there are different subsequent states, since they end
		they are different symbols. And if we have the next character not / and not *, then we are all that we have
		write to the stack (/) as a token of type oper, clear the stack and return to the main state.
	*/
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
	if err := json.Unmarshal([]byte(states), &data); err != nil {
		fmt.Println(err)
	}
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
		fmt.Println(state2int)
		for skey, sval := range istate {
			var val uint32
			if sval[0] == `error` {
				val = 0xff0000
			} else {
				val = uint32(state2int[sval[0]] << 16) // new state
			}
			val |= lexeme[sval[1]] << 8 // lexeme
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
	err := os.WriteFile("../lex_table.go", []byte(out), 0644)
	if err != nil {
		fmt.Println(err.Error())
	}

}
