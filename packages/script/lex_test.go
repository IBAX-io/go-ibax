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
	}
	return
}

func TestLexParser(t *testing.T) {
	test := []TestLexem{
		{" my.test tail...) func 1 ...", "[4 my][11777 46][4 test][4 tail][4872 19][10497 41][520 2][3 1][4872 19]"},
		{"`my string` \"another String\"" + `"test \"subtest\" test"`, "[6 my string][6 another String][6 test \"subtest\" test]"},
		{"contract my { func init {}}", "[264 1][4 my][31489 123][520 2][4 init][31489 123][32001 125][32001 125]"},
		{`callfunc( 1, name + 10)`, `[4 callfunc][10241 40][3 1][11265 44][4 name][2 43][3 10][10497 41]`},
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
