/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpserver

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

// MaxBlock sends the last block ID
// blocksCollection daemon sends this request
func MaxBlock() (*network.MaxBlockResponse, error) {
	infoBlock := &sqldb.InfoBlock{}
	found, err := infoBlock.Get()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting cur blockID")
		return nil, utils.ErrInfo(err)
	}
	if !found {
		log.WithFields(log.Fields{"type": consts.NotFound}).Debug("Can't found info block")
	}

	return &network.MaxBlockResponse{
		BlockID: infoBlock.BlockID,
	}, nil
}
