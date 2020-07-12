/*---------------------------------------------------------------------------------------------
package daemonsctl

import (
	"context"

	"github.com/IBAX-io/go-ibax/packages/modes"
)

// RunAllDaemons start daemons, load contracts and tcpserver
func RunAllDaemons(ctx context.Context) error {
	loader := modes.GetDaemonLoader()

	return loader.Load(ctx)
}
