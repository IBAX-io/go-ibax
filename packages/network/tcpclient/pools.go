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
	pools map[int64]*sync.Pool
}

func (p *bytePool) Get(size int64) []byte {
	power := powerOfTwo(int(size))
	if pool, ok := p.pools[power]; ok {
		return pool.Get().([]byte)
	}

	pool := &sync.Pool{
		New: func() any { return make([]byte, power) },
	}

	p.pools[power] = pool
	return pool.Get().([]byte)
}

func (p *bytePool) Put(buf []byte) {
	if len(buf) == 0 {
		return
	}

	if pool, ok := p.pools[int64(len(buf))]; ok {
		pool.Put(buf)
	}
}
