/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

var firstEcosystemDataSQL = `
INSERT INTO "1_ecosystems" ("id", "name", "is_valued", "digits", "token_symbol", "token_name") VALUES 
	(next_id('1_ecosystems'), 'platform ecosystem', '1', '{{.Digits}}', '{{.TokenSymbol}}', '{{.TokenName}}')
;

INSERT INTO "1_applications" (id, name, conditions, ecosystem) VALUES (next_id('1_applications'), 'System', 'ContractConditions("MainCondition")', '1');
`
