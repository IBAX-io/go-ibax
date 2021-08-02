/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpserver
	block := &model.Block{}

	var blocks []model.Block
	var err error
	if request.ReverseOrder {
		blocks, err = block.GetReverseBlockchain(int64(request.BlockID), network.BlocksPerRequest)
	} else {
		blocks, err = block.GetBlocksFrom(int64(request.BlockID-1), "ASC", network.BlocksPerRequest)
	}

	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err, "block_id": request.BlockID}).Error("Error getting 1000 blocks from block_id")
		if err := network.WriteInt(0, w); err != nil {
			log.WithFields(log.Fields{"type": consts.NetworkError, "error": err}).Error("on sending 0 requested blocks")
		}
		return err
	}

	if err := network.WriteInt(int64(len(blocks)), w); err != nil {
		log.WithFields(log.Fields{"type": consts.NetworkError, "error": err}).Error("on sending requested blocks count")
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
