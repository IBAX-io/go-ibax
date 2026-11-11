/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"errors"
	"os"
	"syscall"
)

// fdatasync syncs the data of f, not its times
func fdatasync(f *os.File) error { return syscall.Fdatasync(int(f.Fd())) }

// openDirect opens path for writes with O_DIRECT|O_DSYNC; nil if its file system does not take
// O_DIRECT
func openDirect(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_DIRECT|syscall.O_DSYNC, 0)
	if errors.Is(err, syscall.EINVAL) {
		return nil, nil
	}
	return f, err
}
