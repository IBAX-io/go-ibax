/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"testing"
)

func TestRegexpMatch(t *testing.T) {
	type args struct {
		str string
		reg string
	}
		//{"url", args{"http://www.google.com", `(https?)://[-A-Za-z0-9+&@#/%?=~_|!:,.;]+[-A-Za-z0-9+&@#/%=~_|]`}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RegexpMatch(tt.args.str, tt.args.reg); got != tt.want {
				t.Errorf("RegexpMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}
