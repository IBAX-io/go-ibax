/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"errors"

	"github.com/IBAX-io/go-ibax/packages/bft"
)

// ShellOf is the shell the proposer of the block of the hash at the height streams
func ShellOf(binData []byte, height int64, hash bft.Hash) ([]byte, error) {
	b, err := parseProposal(binData, height, hash)
	if err != nil {
		return nil, err
	}
	return b.shell()
}

// FailHeldCommit makes the commit of the trial the node holds fail: its transaction breaks a
// deferred constraint, which PostgreSQL checks as it commits
func FailHeldCommit() error {
	held.Lock()
	defer held.Unlock()
	if held.t == nil {
		return errors.New("the node holds no trial")
	}
	if err := held.t.wait(); err != nil {
		return err
	}
	return held.t.dbTx.Connection().Exec(`CREATE TEMP TABLE held_commit_fails (id int UNIQUE DEFERRABLE INITIALLY DEFERRED) ON COMMIT DROP;
		INSERT INTO held_commit_fails VALUES (1), (1)`).Error
}
