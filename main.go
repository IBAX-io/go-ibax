/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package main

import (
	"fmt"
)

func main() {
	runtime.LockOSThread()
	consts.BuildInfo = fmt.Sprintf("%s-%s %s", buildBranch, commitHash, buildDate)
	cmd.Execute()
}
