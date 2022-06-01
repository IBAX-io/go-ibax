/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

import (
	"fmt"
	"os"
	"testing"
)

func TestGetEcosystemScript(t *testing.T) {
	str := fmt.Sprintf(GetFirstEcosystemScript(SqlData{Wallet: -1744264011260937456}))
	path, _ := os.Getwd()
	os.WriteFile(path+"/eco.sql", []byte(str), 0777)
}
