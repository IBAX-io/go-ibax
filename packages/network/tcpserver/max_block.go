/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpserver

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/model"
	}
	if !found {
		log.WithFields(log.Fields{"type": consts.NotFound}).Debug("Can't found info block")
	}

	return &network.MaxBlockResponse{
		BlockID: infoBlock.BlockID,
	}, nil
}
