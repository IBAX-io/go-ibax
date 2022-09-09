/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

import (
	"fmt"
	"testing"
)

type TestLexeme struct {
	Input  string
	Output string
}

func (lexemes Lexemes) String(source []rune) (ret string) {
	for _, item := range lexemes {
		//		slex := string(source[item.Offset:item.Right])
		if item.Type == 0 {
			item.Value = `error`
		}
		ret += fmt.Sprintf("[%d %v]", item.Type, item.Value)
	}
	return
}

func TestLexParser(t *testing.T) {
	test := []TestLexeme{
		{" my.test tail...) func 1 ...", "[4 my][11777 46][4 test][4 tail][5128 20][10497 41][520 2][3 1][5128 20]"},
		{"`my string` \"another String\"" + `"test \"subtest\" test"`, "[6 my string][6 another String][6 test \"subtest\" test]"},
		{"contract my { func init {}}", "[264 1][4 my][31489 123][520 2][4 init][31489 123][32001 125][32001 125]"},
		{`callfunc( 1, name + 10)`, `[4 callfunc][10241 40][3 1][11265 44][4 name][2 43][3 10][10497 41]`},
		{`(ab <= 24 )|| (12>67) && (56==78)`, `[10241 40][4 ab][2 15421][3 24][10497 41][2 31868][10241 40][3 12][2 62][3 67][10497 41][2 9766][10241 40][3 56][2 15677][3 78][10497 41]`},
		{`!ab < !b && 12>=56 && qwe!=asd`, `[2 33][4 ab][2 60][2 33][4 b][2 9766][3 12][2 15933][3 56][2 9766][4 qwe][2 8509][4 asd]`},
		{`ab || 12 && 56`, `[4 ab][2 31868][3 12][2 9766][3 56]`},
		{"12 /*rue \n weweswe*/ 42", `[3 12][3 42]`},
		{`true | 42`, `unknown lexeme   [Ln:1 Col:7]`},
		{"(\r\n)\x03 -", "unknown lexeme  [Ln:2 Col:3]"},
		{` +( - )	/ + // edeld lklm  3edwd`, `[2 43][10241 40][2 45][10497 41][2 47][2 43]`},
		{`23+13424 * 1000.01 test`, `[3 23][2 43][3 13424][2 42][3 1000.01][4 test]`},
		{` 0785/67+iname*(56-31)`, `[3 785][2 47][3 67][2 43][4 iname][2 42][10241 40][3 56][2 45][3 31][10497 41]`},
		{`myvar_45 - a_qwe + t81you - 345rt`, `unknown lexeme r [Ln:1 Col:32]`},
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
