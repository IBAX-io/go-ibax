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
	tests := []struct {
		name string
		args args
		want bool
	}{
		// TODO: Add test cases.
		t.Run(tt.name, func(t *testing.T) {
			if got := RegexpMatch(tt.args.str, tt.args.reg); got != tt.want {
				t.Errorf("RegexpMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}
