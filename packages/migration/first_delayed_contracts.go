/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

var firstDelayedContractsDataSQL = `INSERT INTO "1_delayed_contracts"
		("id", "contract", "key_id", "block_id", "every_block", "high_rate", "conditions")
