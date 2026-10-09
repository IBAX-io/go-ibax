/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

import (
	"strings"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

// The first ecosystem's script builds and registers the founder's wallet
func TestGetEcosystemScript(t *testing.T) {
	const wallet = -1744264011260937456
	script, err := GetFirstEcosystemScript(SqlData{Wallet: wallet})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, converter.Int64ToStr(wallet)) {
		t.Errorf("the script does not mention wallet %d", wallet)
	}
}
