//go:build b7check

/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

// bindCheck makes each run of the program of a plan bind the values with the binder too, and
// panics when the two differ
const bindCheck = true
