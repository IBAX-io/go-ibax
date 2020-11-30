/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
)

// Type10 sends the last block ID
// blocksCollection daemon sends this request
func Type10() (*network.MaxBlockResponse, error) {
	infoBlock := &model.InfoBlock{}
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
