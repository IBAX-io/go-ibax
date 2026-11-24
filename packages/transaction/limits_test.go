/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/smart"
)

// The time left to a transaction of a generated block runs from the start of the block; it never
// is below 0 nor above the time of a block, and the time stops a transaction by ErrTimeLeft
func TestTimeLeft(t *testing.T) {
	limits := NewLimits(letGenBlock)
	var block *timeBlockLimit
	for _, l := range limits.Limiters {
		if bl, ok := l.(*timeBlockLimit); ok {
			block = bl
		}
	}
	if block == nil {
		t.Fatal("no time limit of the block")
	}
	block.Limit = 2 * time.Second
	near := func(left, want int64) bool { return left <= want && left > want-100 }

	block.Start = time.Now().Add(-500 * time.Millisecond)
	if left := block.left().Milliseconds(); !near(left, 1500) {
		t.Fatalf("%d ms left 500 ms in a block of 2000 ms, want 1500", left)
	}
	block.Start = time.Now().Add(-3 * time.Second)
	if left := block.left().Milliseconds(); left != 0 {
		t.Fatalf("%d ms left past the time of the block, want 0", left)
	}
	if err := block.check(nil, letGenBlock); !errors.Is(err, ErrTimeLeft) {
		t.Fatalf("a block past its time goes on: %v", err)
	}
	block.Start = time.Now().Add(time.Second)
	if left := block.left().Milliseconds(); left != 2000 {
		t.Fatalf("%d ms left in a block that starts later, want 2000", left)
	}

	if !errors.Is(ErrTimeLeft, ErrLimitStop) {
		t.Fatal("ErrTimeLeft is not ErrLimitStop")
	}
}

// The check of a proposal of an ordered chain takes the counts and the sizes of the block, not its
// time nor the fuel, which only the execution knows
func TestLimitsOfOrder(t *testing.T) {
	var kinds []string
	for _, l := range NewLimits(GetLetOrder()).Limiters {
		kinds = append(kinds, strings.TrimPrefix(fmt.Sprintf("%T", l), "*transaction."))
	}
	want := []string{"txMaxSize", "txUserLimit", "txMaxLimit", "txUserEcosysLimit"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("the order checks %v, want %v", kinds, want)
	}
}

// The execution of an ordered block checks the fuel alone; a transaction past the fuel of a
// transaction or of the block fails by ErrFuelLimit with the text of the virtual machine, and takes
// no fuel of the block, so that the transactions after it run on the fuel left
func TestFuelLimitOfExecution(t *testing.T) {
	limits := NewLimits(letExecute)
	if len(limits.Limiters) != 1 {
		t.Fatalf("the execution checks %d limits, want the fuel alone", len(limits.Limiters))
	}
	fuel, ok := limits.Limiters[0].(*txMaxFuel)
	if !ok {
		t.Fatalf("the execution checks %T, want the fuel", limits.Limiters[0])
	}
	fuel.LimitTx, fuel.LimitBlock = 100, 250
	tx := func(f int64) *SmartTransactionParser {
		return &SmartTransactionParser{SmartContract: &smart.SmartContract{TxFuel: f}}
	}
	for i, c := range []struct {
		fuel int64
		ok   bool
		used int64
	}{
		{90, true, 90}, {101, false, 90}, {90, true, 180}, {80, false, 180}, {70, true, 250}, {1, false, 250},
	} {
		err := fuel.check(tx(c.fuel), letExecute)
		if c.ok != (err == nil) || (err != nil && !errors.Is(err, ErrFuelLimit)) || fuel.Fuel != c.used {
			t.Fatalf("transaction %d of fuel %d: %v, the block used %d, want %d", i, c.fuel, err, fuel.Fuel, c.used)
		}
		if err != nil && !strings.Contains(err.Error(), "Max fuel") {
			t.Fatalf("transaction %d: the text %q is not that of the virtual machine", i, err)
		}
	}
}
