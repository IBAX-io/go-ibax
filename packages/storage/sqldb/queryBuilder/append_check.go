//go:build b7check

/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package queryBuilder

// appendCheck makes each bound statement compare its values with fmt %q of them too, and panic
// when the two differ
const appendCheck = true
