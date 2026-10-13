/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

import (
	"os"
	"testing"
)

func TestGetEcosystemScript(t *testing.T) {
	str, err := GetFirstEcosystemScript(SqlData{Wallet: -1744264011260937456})
	if err != nil {
		t.Fatal(err)
	}
	path, _ := os.Getwd()
	os.WriteFile(path+"/eco.sql", []byte(str), 0777)
}
