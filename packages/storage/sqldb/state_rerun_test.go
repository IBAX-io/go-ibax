package sqldb

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIsRerun checks the errors after which a transaction runs again are told through the
// wrappings of the queries, and the others are not
func TestIsRerun(t *testing.T) {
	require.True(t, IsRerun(ErrNeedsOrder))
	require.True(t, IsRerun(errRestart))
	require.True(t, IsRerun(fmt.Errorf("%w in query SELECT 1 []", fmt.Errorf("%w in query SELECT 1 []", errRestart))))
	require.False(t, IsRerun(errors.New(ErrNeedsOrder.Error())))
	require.False(t, IsRerun(nil))
}
