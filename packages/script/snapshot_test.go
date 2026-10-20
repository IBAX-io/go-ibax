/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package script

import (
	"fmt"
	"slices"
	"testing"
)

// vmState is what Restore must put back: every object by name with its value, every child by
// index, and the owner of every contract
func vmState(vm *VM) string {
	names := make([]string, 0, len(vm.Objects))
	for k := range vm.Objects {
		names = append(names, k)
	}
	slices.Sort(names)
	out := ""
	for _, k := range names {
		o := vm.Objects[k]
		out += fmt.Sprintf("%s %p %d %p\n", k, o, o.Type, o.Value)
	}
	for i, c := range vm.Children {
		out += fmt.Sprintf("child %d %p", i, c)
		if c != nil && c.Type == ObjectType_Contract {
			out += fmt.Sprintf(" %+v", *c.GetContractInfo().Owner)
		}
		out += "\n"
	}
	return out
}

func callString(t *testing.T, vm *VM, name string) string {
	t.Helper()
	out, err := vm.Call(name, nil, map[string]any{`rt_state`: uint32(1)})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return fmt.Sprint(out[0])
}

// A block that edits a function and a contract, adds a contract and changes the owners leaves the
// virtual machine, after Restore, as it was before the block
func TestVMSnapshot(t *testing.T) {
	vm := NewVM()
	owner := &OwnerInfo{StateID: 1, Active: true, TableID: 1}
	for _, code := range []string{
		`func Word() string { return "before" }`,
		`contract Kept { action { } }`,
		`contract Edited { action { } }`,
	} {
		if err := vm.Compile([]rune(code), owner); err != nil {
			t.Fatal(err)
		}
	}
	before := vmState(vm)
	if got := callString(t, vm, `Word`); got != "before" {
		t.Fatalf("Word = %s", got)
	}

	snap := vm.Snapshot()
	for _, code := range []string{
		`func Word() string { return "after" }`,
		`contract Edited { data { Value int } action { } }`,
		`contract Added { action { } }`,
	} {
		root, err := vm.CompileBlock([]rune(code), &OwnerInfo{StateID: 1, Active: true, TableID: 2})
		if err != nil {
			t.Fatal(err)
		}
		vm.FlushBlock(root)
	}
	for _, c := range vm.Children {
		if c != nil && c.Type == ObjectType_Contract {
			c.GetContractInfo().Owner.Active = false
			c.GetContractInfo().Owner.WalletID = 9
		}
	}
	if got := callString(t, vm, `Word`); got != "after" {
		t.Fatalf("Word after the block = %s", got)
	}
	if _, ok := vm.Objects[`@1Added`]; !ok {
		t.Fatal("the block did not add the contract")
	}
	if vmState(vm) == before {
		t.Fatal("the block changed nothing")
	}

	vm.Restore(snap)
	if got := vmState(vm); got != before {
		t.Fatalf("after Restore:\n%s\nbefore:\n%s", got, before)
	}
	if got := callString(t, vm, `Word`); got != "before" {
		t.Fatalf("Word after Restore = %s", got)
	}
	if _, ok := vm.Objects[`@1Added`]; ok {
		t.Fatal("the added contract is still there")
	}

	// the snapshot is a copy: what the next block changes does not reach it
	root, err := vm.CompileBlock([]rune(`contract Added { action { } }`), owner)
	if err != nil {
		t.Fatal(err)
	}
	vm.FlushBlock(root)
	vm.Restore(snap)
	if got := vmState(vm); got != before {
		t.Fatalf("after the second Restore:\n%s\nbefore:\n%s", got, before)
	}
}
