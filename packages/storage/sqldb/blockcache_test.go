/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A BlockCache reads a key once until a block commits or DBConn changes, keeps neither failed
// reads nor those it is told not to keep, nor a read a commit overtook, and starts again when full
func TestBlockCache(t *testing.T) {
	var c BlockCache[int, int]
	loads := 0
	load := func(v int) func() (int, bool, error) {
		return func() (int, bool, error) { loads++; return v, true, nil }
	}

	v, err := c.Get(1, load(10))
	require.NoError(t, err)
	require.Equal(t, 10, v)
	v, _ = c.Get(1, load(11))
	require.Equal(t, 10, v)
	require.Equal(t, 1, loads)

	commits.Add(1)
	v, _ = c.Get(1, load(12))
	require.Equal(t, 12, v)
	require.Equal(t, 2, loads)

	saved := DBConn
	DBConn = &gorm.DB{}
	v, _ = c.Get(1, load(13))
	DBConn = saved
	require.Equal(t, 13, v)
	v, _ = c.Get(1, load(14))
	require.Equal(t, 14, v)

	failed := errors.New("failed")
	_, err = c.Get(2, func() (int, bool, error) { return 0, true, failed })
	require.ErrorIs(t, err, failed)
	v, _ = c.Get(2, func() (int, bool, error) { return 20, false, nil })
	require.Equal(t, 20, v)
	v, _ = c.Get(2, load(21))
	require.Equal(t, 21, v)

	v, _ = c.Get(3, func() (int, bool, error) { commits.Add(1); return 30, true, nil })
	require.Equal(t, 30, v)
	v, _ = c.Get(3, load(31))
	require.Equal(t, 31, v)
	v, _ = c.Get(3, load(32))
	require.Equal(t, 31, v)

	for k := range blockCacheSize {
		c.Get(100+k, load(k))
	}
	v, _ = c.Get(3, load(33))
	require.Equal(t, 33, v, "a full cache starts again")
}
