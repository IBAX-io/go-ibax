/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package gencache is a bounded map of two generations: puts go to the current generation, and a
// full current generation becomes the previous one, dropping what the previous one held. A get
// finds what either generation holds and moves an entry of the previous generation to the
// current one, so the entries in use stay and the cache holds at most twice its size.
package gencache

import "sync"

// Cache is safe for concurrent use
type Cache[K comparable, V any] struct {
	mu   sync.Mutex
	size int
	cur  map[K]V
	prev map[K]V
}

// New is a cache of generations of size entries
func New[K comparable, V any](size int) *Cache[K, V] {
	return &Cache[K, V]{size: size, cur: make(map[K]V, size), prev: map[K]V{}}
}

// Get is the value of a key
func (c *Cache[K, V]) Get(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.cur[k]; ok {
		return v, true
	}
	v, ok := c.prev[k]
	if ok {
		delete(c.prev, k)
		c.put(k, v)
	}
	return v, ok
}

// Put sets the value of a key
func (c *Cache[K, V]) Put(k K, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.put(k, v)
}

func (c *Cache[K, V]) put(k K, v V) {
	if _, ok := c.cur[k]; !ok && len(c.cur) >= c.size {
		c.prev = c.cur
		c.cur = make(map[K]V, c.size)
	}
	c.cur[k] = v
}

// Len is the entries of both generations
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.cur) + len(c.prev)
}
