/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpclient

import (
	"sync"
)

// return nearest power of 2 that bigest than v
func powerOfTwo(v int) int64 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++
	return int64(v)
}

var BytesPool *bytePool

func init() {
	BytesPool = &bytePool{
		pools: make(map[int64]*sync.Pool),
	}
}

type bytePool struct {
	mu    sync.Mutex
	pools map[int64]*sync.Pool
}

func (p *bytePool) Get(size int64) []byte {
	power := powerOfTwo(int(size))
	p.mu.Lock()
	pool, ok := p.pools[power]
	if !ok {
		pool = &sync.Pool{
			New: func() any { return make([]byte, power) },
		}
		p.pools[power] = pool
	}
	p.mu.Unlock()
	return pool.Get().([]byte)
}

func (p *bytePool) Put(buf []byte) {
	if len(buf) == 0 {
		return
	}

	p.mu.Lock()
	pool, ok := p.pools[int64(len(buf))]
	p.mu.Unlock()
	if ok {
		pool.Put(buf)
	}
}
