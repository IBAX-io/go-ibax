/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package clb

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
)

var keysDataSQL = `
INSERT INTO "1_keys" (id, account, pub, blocked, ecosystem) 
VALUES (` + consts.GuestKey + `, '` + consts.GuestAddress + `', decode('` + consts.GuestPublic + `', 'hex'), 1, '%[1]d');
`
