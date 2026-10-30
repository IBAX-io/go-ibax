/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"testing"

	"github.com/vmihailenco/msgpack/v5"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// The first block of an earlier binary, which has no chain format, reads as format 0 and is
// refused; one of the format of the node is taken
func TestCheckChainFormat(t *testing.T) {
	earlier, err := msgpack.Marshal(&struct {
		KeyID          int64
		Time           int64
		HonorNodesOnly uint64
	}{KeyID: -5000, Time: 1, HonorNodesOnly: 1})
	if err != nil {
		t.Fatal(err)
	}
	var first types.FirstBlock
	if err := msgpack.Unmarshal(earlier, &first); err != nil {
		t.Fatal(err)
	}
	if first.KeyID != -5000 || first.ChainFormat != 0 {
		t.Fatalf("the first block of an earlier binary read as %+v", first)
	}
	if err := CheckChainFormat(&first); err == nil {
		t.Fatal("the first block of an earlier binary is taken")
	}
	if err := CheckChainFormat(&types.FirstBlock{ChainFormat: consts.ChainFormat + 1}); err == nil {
		t.Fatal("a first block of a later format is taken")
	}
	if err := CheckChainFormat(&types.FirstBlock{ChainFormat: consts.ChainFormat}); err != nil {
		t.Fatal(err)
	}
}
