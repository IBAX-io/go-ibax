/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
)

var membersDataSQL = `
	INSERT INTO "1_members" ("id", "account", "member_name", "ecosystem") 
	VALUES
		(next_id('1_members'), '{{.Account}}', 'founder', '{{.Ecosystem}}'),
		(next_id('1_members'), '` + consts.GuestAddress + `', 'guest', '{{.Ecosystem}}'),
		(next_id('1_members'), '` + converter.HoleAddrMap[converter.BlackHoleAddr].S + `', 'black_hole', '{{.Ecosystem}}'),
		(next_id('1_members'), '` + converter.HoleAddrMap[converter.WhiteHoleAddr].S + `', 'white_hole', '{{.Ecosystem}}');
`
