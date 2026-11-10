//go:build !linux

/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import "os"

// fdatasync syncs f: the systems besides Linux the node builds for sync the data with the file
func fdatasync(f *os.File) error { return f.Sync() }
