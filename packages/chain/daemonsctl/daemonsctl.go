/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemonsctl

import (
	"context"

	"github.com/IBAX-io/go-ibax/packages/modes"
)

// RunAllDaemons start daemons, load contracts and tcpserver
func RunAllDaemons(ctx context.Context) error {
	return modes.GetDaemonLoader().Load(ctx)
}
