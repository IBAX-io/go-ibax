/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package syspar

import "testing"

// GetPriceExec finds a price by the full name of its parameter, and an absent or empty parameter
// as the two lookups and the conversion it replaced did
func TestGetPriceExec(t *testing.T) {
	mutex.Lock()
	saved := cache
	cache = map[string]string{PriceExec + "db_find": "25", PriceExec + "empty": "", PriceExec + "bad": "x"}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		cache = saved
		mutex.Unlock()
	}()
	for _, c := range []struct {
		param string
		price int64
		ok    bool
	}{{PriceExec + "db_find", 25, true}, {PriceExec + "empty", 0, true}, {PriceExec + "bad", 0, true}, {PriceExec + "absent", 0, false}} {
		if price, ok := GetPriceExec(c.param); price != c.price || ok != c.ok {
			t.Errorf("%s: %d %v, want %d %v", c.param, price, ok, c.price, c.ok)
		}
	}
}
