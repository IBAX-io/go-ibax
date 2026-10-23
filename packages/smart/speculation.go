/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"errors"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

// ErrSerialOnly stops the speculative run of a transaction that changes the state of the node,
// which the block then runs in its own transaction
var ErrSerialOnly = errors.New("the transaction runs in the transaction of the block")

// serialOnly stops a speculative run before what it names changes the state of the node: the
// contracts of the VM, the parameters, the languages, the schema of the tables
func (sc *SmartContract) serialOnly(what string) error {
	if sc.DbTransaction.Speculating() {
		sc.DbTransaction.MarkSerial(what)
		return ErrSerialOnly
	}
	return nil
}

// unusedOutputs are the outputs of the account the transaction has not spent, a read of the
// outputs of the account
func (sc *SmartContract) unusedOutputs(key sqldb.KeyUTXO, outputs map[sqldb.KeyUTXO][]sqldb.SpentInfo) []sqldb.SpentInfo {
	sc.DbTransaction.RecordRead(sqldb.UTXORow(key))
	return sqldb.GetUnusedOutputsMap(key, outputs)
}
