/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package conf

import "runtime"

// WorkerCount is the number of the workers that run the transactions of a block in parallel
func (c ExecConfig) WorkerCount() int {
	if c.Workers > 0 {
		return c.Workers
	}
	return runtime.NumCPU()
}
