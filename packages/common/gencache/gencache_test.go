/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package gencache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerations(t *testing.T) {
	c := New[int, string](2)
	c.Put(1, "a")
	c.Put(2, "b")
	c.Put(3, "c") // 1 and 2 are the previous generation
	v, ok := c.Get(1)
	assert.True(t, ok)
	assert.Equal(t, "a", v)
	c.Put(4, "d") // the current generation 3, 1 is full: it becomes the previous one, 2 goes
	_, ok = c.Get(2)
	assert.False(t, ok)
	for _, k := range []int{1, 3, 4} {
		_, ok = c.Get(k)
		assert.True(t, ok, k)
	}
	assert.LessOrEqual(t, c.Len(), 4)
}
