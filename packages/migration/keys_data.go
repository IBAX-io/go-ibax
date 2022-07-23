/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

import (
	"strconv"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
)

var keysDataSQL = `
INSERT INTO "1_keys" (id, account, pub, blocked, ecosystem) 
VALUES 
	(` + consts.GuestKey + `, '` + consts.GuestAddress + `', decode('` + consts.GuestPublic + `', 'hex'), 1, '{{.Ecosystem}}'),
	(` + strconv.FormatInt(converter.HoleAddrMap[converter.BlackHoleAddr].K, 10) + `, '` + converter.HoleAddrMap[converter.BlackHoleAddr].S + `', decode('', 'hex'), 1, '{{.Ecosystem}}'),
	(` + strconv.FormatInt(converter.HoleAddrMap[converter.WhiteHoleAddr].K, 10) + `, '` + converter.HoleAddrMap[converter.WhiteHoleAddr].S + `', decode('', 'hex'), 1, '{{.Ecosystem}}');
`
