/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"errors"
	"time"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

var (
	// ErrNewUserPending: the key is not registered yet, its NewUser transaction is not in a block
	ErrNewUserPending = errors.New("The block packing in progress, please wait")
	// ErrNewUserFailed: the NewUser transaction failed and the key is not registered
	ErrNewUserFailed = errors.New("encountered some problems when login account")
)

const newUserPollInterval = 200 * time.Millisecond

// AwaitNewUser waits until the key of account address wallet is registered by a login's NewUser
// transaction (hash), and loads it into account. What counts is the key, not the transaction:
// logins of one new key made at once each queue a NewUser transaction (the same one within a
// second), the first in a block registers the key, the others fail, and all those logins succeed.
func AwaitNewUser(account *sqldb.Key, wallet int64, hash []byte, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		// The log first: a block that failed this transaction has committed whichever registered the key
		logged := &sqldb.LogTransaction{}
		inBlock, err := logged.GetByHash(nil, hash)
		if err != nil {
			return err
		}
		found, err := account.Get(nil, wallet)
		if err != nil {
			return err
		}
		if found && len(account.PublicKey) > 0 {
			return nil
		}
		if inBlock && logged.Status != 0 {
			return ErrNewUserFailed
		}
		if time.Now().After(deadline) {
			return ErrNewUserPending
		}
		time.Sleep(newUserPollInterval)
	}
}
