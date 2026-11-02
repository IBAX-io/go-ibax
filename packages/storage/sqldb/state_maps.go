/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"sync"
	"sync/atomic"
)

// smap is a map by string that the views read without a lock while the thread that commits, and
// only it, writes it. A view may see a key as it was before or after a commit it runs along: the
// seq it read before (BlockState.pub) is older than the commit, whose event then fails the read.
type smap[V any] struct {
	m sync.Map
	n atomic.Int64
}

func (s *smap[V]) load(k string) (V, bool) {
	v, ok := s.m.Load(k)
	if !ok {
		var zero V
		return zero, false
	}
	return v.(V), true
}

func (s *smap[V]) store(k string, v V) {
	if _, loaded := s.m.Swap(k, v); !loaded {
		s.n.Add(1)
	}
}

func (s *smap[V]) delete(k string) {
	if _, loaded := s.m.LoadAndDelete(k); loaded {
		s.n.Add(-1)
	}
}

func (s *smap[V]) len() int { return int(s.n.Load()) }

// each calls f for each key until f returns false
func (s *smap[V]) each(f func(k string, v V) bool) {
	s.m.Range(func(k, v any) bool { return f(k.(string), v.(V)) })
}

func (s *smap[V]) clear() {
	s.m.Clear()
	s.n.Store(0)
}
