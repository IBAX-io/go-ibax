/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
INSERT INTO "1_keys" (id, account, pub, blocked, ecosystem) 
VALUES (` + consts.GuestKey + `, '` + consts.GuestAddress + `', decode('` + consts.GuestPublic + `', 'hex'), 1, '{{.Ecosystem}}');
`
