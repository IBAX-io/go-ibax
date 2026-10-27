/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"testing"

	"github.com/shopspring/decimal"
)

// The estimate of the fees a payer must hold before its contract runs is the sum of the fee
// categories but the compute cost, which the run itself is given. GetEstimate dropped the sum, so
// the pre-run check of checkVerify refused no one.
func TestGetEstimate(t *testing.T) {
	pay := &PaymentInfo{FuelCategories: []FuelCategory{
		NewFuelCategory(FuelType_storage_fee, decimal.NewFromInt(10), 0, 0),
		NewFuelCategory(FuelType_vmCost_fee, decimal.NewFromInt(100), 0, 0),
		NewFuelCategory(FuelType_expedite_fee, decimal.NewFromInt(20), 0, 0),
	}}
	if got := pay.GetEstimate(); !got.Equal(decimal.NewFromInt(30)) {
		t.Errorf("GetEstimate = %s, want 30", got)
	}
}
