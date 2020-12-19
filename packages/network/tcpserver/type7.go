/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpserver

import (
	"net"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/network"

	log "github.com/sirupsen/logrus"
)

// Type7 writes the body of the specified block
// blocksCollection and queue_parser_blocks daemons send the request through p.GetBlocks()
func Type7(request *network.GetBodiesRequest, w net.Conn) error {
	block := &model.Block{}

	var blocks []model.Block
	var err error
		return err
	}

	if err := network.WriteInt(lenOfBlockData(blocks), w); err != nil {
		log.WithFields(log.Fields{"type": consts.NetworkError, "error": err}).Error("on sending requested blocks data length")
		return err
	}

	for _, b := range blocks {
		br := &network.GetBodyResponse{Data: b.Data}
		if err := br.Write(w); err != nil {
			return err
		}
	}

	return nil
}

func lenOfBlockData(blocks []model.Block) int64 {
	var length int64
	for i := 0; i < len(blocks); i++ {
		length += int64(len(blocks[i].Data))
	}

	return length
}
