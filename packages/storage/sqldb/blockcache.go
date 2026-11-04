/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"sync"
	"sync/atomic"

	"gorm.io/gorm"
)

// commits counts the transactions StartTransaction began that committed: the blocks, their
// rollbacks and the BFT rounds, which are all that write the state while the node serves
var commits atomic.Uint64

// Commits is the number of the transactions of the blocks committed so far; what the blocks
// wrote is the same while it is
func Commits() uint64 { return commits.Load() }

// blockCacheSize bounds the values a BlockCache keeps; past it the cache starts again
const blockCacheSize = 4096

// BlockCache keeps the values read from the state through DBConn, a key each, until a
// transaction of the blocks commits or DBConn changes. A value read while one commits is kept no
// longer than its commit.
type BlockCache[K comparable, V any] struct {
	mu     sync.Mutex
	at     cacheAt
	values map[K]V
}

// cacheAt is the state a BlockCache keeps values of
type cacheAt struct {
	commits uint64
	db      *gorm.DB
}

func cacheNow() cacheAt { return cacheAt{commits: Commits(), db: DBConn} }

// Get is the value of the key, which load reads when the cache does not have it; a value load
// fails to read is not kept, nor is one it tells not to keep
func (c *BlockCache[K, V]) Get(key K, load func() (V, bool, error)) (V, error) {
	at := cacheNow()
	c.mu.Lock()
	if c.at == at {
		if v, ok := c.values[key]; ok {
			c.mu.Unlock()
			return v, nil
		}
	}
	c.mu.Unlock()
	v, keep, err := load()
	if err != nil || !keep {
		return v, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.values == nil || c.at != at || len(c.values) >= blockCacheSize {
		if cacheNow() != at {
			// a block committed while load read: the value may be of the state before it
			return v, nil
		}
		c.at, c.values = at, make(map[K]V)
	}
	c.values[key] = v
	return v, nil
}
