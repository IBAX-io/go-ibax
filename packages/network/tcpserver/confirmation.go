/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpserver

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

// Confirmation writes the hash of the specified block
// The request is sent by 'confirmations' daemon
func Confirmation(r *network.ConfirmRequest) (*network.ConfirmResponse, error) {
	resp := &network.ConfirmResponse{}
	block := &sqldb.BlockChain{}
	found, err := block.Get(int64(r.BlockID))
	if err != nil || !found {
		hash := [32]byte{}
		resp.Hash = hash[:]
	} else {
		resp.Hash = block.Hash // can we send binary data ?
	}
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err, "block_id": r.BlockID}).Error("Getting block")
	} else if len(block.Hash) == 0 {
		log.WithFields(log.Fields{"type": consts.DBError, "block_id": r.BlockID}).Warning("Block not found")
	}
	return resp, nil
}
