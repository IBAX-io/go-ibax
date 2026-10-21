/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package sqldb

import (
	"bytes"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

// A block id past what three bytes hold is announced whole
func TestInfoBlockMarshallBlockID(t *testing.T) {
	hash := bytes.Repeat([]byte{0xab}, 32)
	for _, id := range []int64{1, 1<<24 + 5, 1<<40 + 7} {
		data := (&InfoBlock{BlockID: id, Hash: hash}).Marshall()
		if got := converter.BinToDec(data[:BlockIDSize]); got != id {
			t.Errorf("block id %d is announced as %d", id, got)
		}
		if !bytes.Equal(data[BlockIDSize:], hash) {
			t.Errorf("block %d: hash %x", id, data[BlockIDSize:])
		}
	}
}
