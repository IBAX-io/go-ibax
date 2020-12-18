/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemonsctl

func RunAllDaemons(ctx context.Context) error {
	loader := modes.GetDaemonLoader()

	return loader.Load(ctx)
}
