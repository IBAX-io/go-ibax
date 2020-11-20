/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

import (
	"fmt"
	"testing"
)

type TestLexem struct {
	Input  string
	Output string
}

func (lexems Lexems) String(source []rune) (ret string) {
	for _, item := range lexems {
		//		slex := string(source[item.Offset:item.Right])
		if item.Type == 0 {
			item.Value = `error`
		}
		ret += fmt.Sprintf("[%d %v]", item.Type, item.Value)
		{`ab || 12 && 56`, `[4 ab][2 31868][3 12][2 9766][3 56]`},
		{"12 /*rue \n weweswe*/ 42", `[3 12][3 42]`},
		{`true | 42`, `unknown lexem   [Ln:1 Col:7]`},
		{"(\r\n)\x03 -", "unknown lexem  [Ln:2 Col:3]"},
		{` +( - )	/ + // edeld lklm  3edwd`, `[2 43][10241 40][2 45][10497 41][2 47][2 43]`},
		{`23+13424 * 1000.01 Тест`, `[3 23][2 43][3 13424][2 42][3 1000.01][4 Тест]`},
		{` 0785/67+iname*(56-31)`, `[3 785][2 47][3 67][2 43][4 iname][2 42][10241 40][3 56][2 45][3 31][10497 41]`},
		{`myvar_45 - a_qwe + t81you - 345rt`, `unknown lexem r [Ln:1 Col:32]`},
		{`10 + #mytable[id = 234].name * 20`, `[3 10][2 43][8961 35][4 mytable][23297 91][4 id][15617 61][3 234][23809 93][11777 46][4 name][2 42][3 20]`},
	}
	for _, item := range test {
		source := []rune(item.Input)
		if out, err := lexParser(source); err != nil {
			if err.Error() != item.Output {
				fmt.Println(string(source))
				t.Error(`error of lexical parser ` + err.Error())
			}
		} else if out.String(source) != item.Output {
			t.Error(`error of lexical parser ` + item.Input)
			fmt.Println(out.String(source))
		}
	}
}
