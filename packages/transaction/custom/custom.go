/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package custom
	Init() error
	Validate() error
	Action() error
	Rollback() error
	Header() *tx.Header
}
