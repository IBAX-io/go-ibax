/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/require"
)

func TestStopNetworkAlone(t *testing.T) {
	stop := &transaction.Transaction{Inner: &transaction.StopNetworkParser{Data: &types.StopNetwork{}}}
	first := &transaction.Transaction{Inner: &transaction.FirstBlockParser{Data: &types.FirstBlock{}}}
	require.NoError(t, checkStopNetworkAlone(nil))
	require.NoError(t, checkStopNetworkAlone([]*transaction.Transaction{stop}))
	require.NoError(t, checkStopNetworkAlone([]*transaction.Transaction{first, first}))
	require.ErrorIs(t, checkStopNetworkAlone([]*transaction.Transaction{first, stop}), errStopNetworkNotAlone)
	require.ErrorIs(t, checkStopNetworkAlone([]*transaction.Transaction{stop, first}), errStopNetworkNotAlone)
}
