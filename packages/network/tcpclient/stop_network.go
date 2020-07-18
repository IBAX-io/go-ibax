/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpclient

import (
		Type: network.RequestTypeStopNetwork,
	}

	if err = rt.Write(conn); err != nil {
		return err
	}

	if err = req.Write(conn); err != nil {
		return err
	}

	res := &network.StopNetworkResponse{}
	if err = res.Read(conn); err != nil {
		return err
	}

	if len(res.Hash) != consts.HashSize {
		return network.ErrNotAccepted
	}

	return nil
}
