/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

var firstDelayedContractsDataSQL = `INSERT INTO "1_delayed_contracts"
		("id", "contract", "key_id", "block_id", "every_block", "high_rate", "conditions")
	VALUES
		(next_id('1_delayed_contracts'), '@1CheckNodesBan', '{{.Wallet}}', '10', '10', '4','ContractConditions("@1MainCondition")');
`
