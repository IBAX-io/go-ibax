/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"os"
	"syscall"
)

// fdatasync syncs the data of f, not its times
func fdatasync(f *os.File) error { return syscall.Fdatasync(int(f.Fd())) }
