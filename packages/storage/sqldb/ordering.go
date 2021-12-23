/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

type ordering string

const (
	// OrderASC as ASC
	OrderASC = ordering("ASC")
	// OrderDESC as DESC
	OrderDESC = ordering("DESC")
)
