/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import "sync"

type transactionCache struct {
	mutex sync.RWMutex
	cache map[string]*Transaction
}

func (tc *transactionCache) Get(hash string) (t *Transaction, ok bool) {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	t, ok = tc.cache[hash]
	return
}


func (tc *transactionCache) Clean() {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	tc.cache = make(map[string]*Transaction)
}
