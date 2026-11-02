/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import "github.com/IBAX-io/go-ibax/packages/bft"

// ShellOf is the shell the proposer of the block of the hash at the height streams
func ShellOf(binData []byte, height int64, hash bft.Hash) ([]byte, error) {
	b, err := parseProposal(binData, height, hash)
	if err != nil {
		return nil, err
	}
	return b.shell()
}
