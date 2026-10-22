/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPresentStatuses(t *testing.T) {
	assert.Empty(t, presentStatuses(nil))
	assert.Empty(t, presentStatuses([]*TransactionStatus{nil, nil}))
	a, b := &TransactionStatus{Hash: []byte{1}}, &TransactionStatus{Hash: []byte{2}}
	assert.Equal(t, []*TransactionStatus{a, b}, presentStatuses([]*TransactionStatus{nil, a, nil, b}))
}
