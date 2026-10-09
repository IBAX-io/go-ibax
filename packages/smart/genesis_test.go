/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/stretchr/testify/require"
)

// Every contract of the first ecosystem's genesis compiles, in the order a new node loads them: one
// that does not is left out of the VM, and every contract after it is called by a wrong id
func TestGenesisContractsCompile(t *testing.T) {
	files, err := filepath.Glob("../migration/contracts/first_ecosystem/*.sim")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	sort.Strings(files)

	vm := script.NewVM()
	vm.SetExtendCost(getCost)
	vm.SetFuncCallsDB(funcCallsDBP)
	vm.Extend(&script.ExtendData{
		Objects:    EmbedFuncs(defineVMType()),
		AutoPars:   map[string]string{`*smart.SmartContract`: `sc`},
		WriteFuncs: writeFuncs,
	})
	require.NoError(t, script.LoadSysFuncs(vm, 1))
	for i, file := range files {
		source, err := os.ReadFile(file)
		require.NoError(t, err)
		owner := script.OwnerInfo{StateID: 1, TableID: int64(i + 1)}
		require.NoError(t, vm.Compile([]rune(string(source)), &owner), filepath.Base(file))
	}
}
