/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

// serialOnly stops a run in parallel before what it names changes the state of the node out of
// the database: the contracts of the VM, the parameters, the languages, the schema of the tables,
// the processes of the node. The block runs the transaction again in its order, where it goes on.
func (sc *SmartContract) serialOnly(what string) error {
	if v := sc.DbTransaction.View(); v != nil {
		return v.SetNeedsOrder(what)
	}
	return nil
}

// unusedOutputs are the outputs of the account the block has not spent
func (sc *SmartContract) unusedOutputs(key sqldb.KeyUTXO, outputs map[sqldb.KeyUTXO][]sqldb.SpentInfo) []sqldb.SpentInfo {
	if v := sc.DbTransaction.View(); v != nil {
		return v.UnusedOutputs(key)
	}
	return sqldb.GetUnusedOutputsMap(key, outputs)
}
