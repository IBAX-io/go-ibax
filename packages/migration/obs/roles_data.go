/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
	(next_id('1_roles'),'', 'Chain Consensus asbl', '0', '3', '{}', '{"rids": "1"}', '%[1]d'),
	(next_id('1_roles'),'', 'Candidate for validators', '0', '3', '{}', '{}', '%[1]d'),
	(next_id('1_roles'),'', 'Validator', '0', '3', '{}', '{}', '%[1]d'),
	(next_id('1_roles'),'', 'Investor with voting rights', '0', '3', '{}', '{}', '%[1]d'),
	(next_id('1_roles'),'', 'Delegate', '0', '3', '{}', '{}', '%[1]d');

	INSERT INTO "1_roles_participants" ("id","role" ,"member", "date_created", "ecosystem")
	VALUES (next_id('1_roles_participants'), '{"id": "1", "type": "3", "name": "Admin", "image_id":"0"}', '{"member_id": "%[2]d", "member_name": "founder", "image_id": "0"}', floor(extract(epoch from now())), '%[1]d'),
	(next_id('1_roles_participants'), '{"id": "2", "type": "3", "name": "Developer", "image_id":"0"}', '{"member_id": "%[2]d", "member_name": "founder", "image_id": "0"}', floor(extract(epoch from now())), '%[1]d');

	INSERT INTO "1_members" ("id", "account", "member_name", "ecosystem") 
	VALUES
		(next_id('1_members'), '%[3]s', 'founder', '%[1]d'),
		(next_id('1_members'), '` + consts.GuestAddress + `', 'guest', '%[1]d');

`
