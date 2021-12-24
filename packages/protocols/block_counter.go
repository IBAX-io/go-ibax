/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package protocols

import "github.com/IBAX-io/go-ibax/packages/storage/sqldb"

type intervalBlocksCounter interface {
	count(state blockGenerationState) (int, error)
}

type blocksCounter struct {
}

func (bc *blocksCounter) count(state blockGenerationState) (int, error) {
	blockchain := &sqldb.BlockChain{}
	blocks, err := blockchain.GetNodeBlocksAtTime(state.start, state.start.Add(state.duration), state.nodePosition)
	if err != nil {
		return 0, err
	}
	return len(blocks), nil
}
