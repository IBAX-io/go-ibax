/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package childchain

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
)

var keysDataSQL = `
INSERT INTO "1_keys" (id, account, pub, blocked, ecosystem) 
VALUES (` + consts.GuestKey + `, '` + consts.GuestAddress + `', decode('', 'hex'), 1, '%[1]d');
`
