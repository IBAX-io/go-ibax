/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
VALUES (` + consts.GuestKey + `, '` + consts.GuestAddress + `', decode('` + consts.GuestPublic + `', 'hex'), 1, '{{.Ecosystem}}');
`
