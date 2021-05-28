/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package updates

var M320 = `

INSERT INTO "1_system_parameters" (id, name, value, conditions) VALUES
	(next_id('1_system_parameters'), 'price_exec_is_honor_node_key', '10', 'ContractAccess("@1UpdateSysParam")');
