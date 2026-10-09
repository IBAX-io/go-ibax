/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package updates

// MigrationUTXOMovementsIndex gives a node made before it the index the initial schema now has,
// by which an account's UTXO movements are listed a block at a time; it is the node's own data
var MigrationUTXOMovementsIndex = `
CREATE INDEX IF NOT EXISTS "spent_info_ecosystem_output_key_id_block_id_idx" ON "spent_info" (ecosystem, output_key_id, block_id);
`
