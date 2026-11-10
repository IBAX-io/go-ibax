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

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/stretchr/testify/require"
)

// A node that joins from a snapshot loads the contracts of block 1, puts the machine back as it was
// before block 1, and loads the contracts of the snapshot: each is called by the id of its table,
// as the transactions of the chain name it, and finds the system functions
func TestContractsAfterRestore(t *testing.T) {
	files, err := filepath.Glob("../migration/contracts/first_ecosystem/*.sim")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	sort.Strings(files)
	var list []sqldb.Contract
	for i, file := range files {
		source, err := os.ReadFile(file)
		require.NoError(t, err)
		list = append(list, sqldb.Contract{ID: int64(i + 1), Value: string(source), EcosystemID: 1})
	}

	vm := script.NewVM()
	vm.SetExtendCost(getCost)
	vm.SetFuncCallsDB(funcCallsDBP)
	vm.Extend(extendData(defineVMType()))
	before := vm.Snapshot()
	require.NoError(t, loadContractList(vm, list[:len(list)/2]))
	vm.FlushExtern()

	vm.Restore(before)
	require.NoError(t, loadContractList(vm, list))
	vm.FlushExtern()
	for _, c := range list {
		got := VMGetContractByID(vm, int32(consts.ShiftContractID+c.ID))
		require.NotNil(t, got, "contract %d", c.ID)
		require.Equal(t, c.ID, got.Info().Owner.TableID)
	}
}
