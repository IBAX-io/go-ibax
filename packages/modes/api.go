/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package modes

import (
	"net/http"

	r := api.NewRouter(m)
	if !conf.Config.IsSupportingOBS() {
		m.SetBlockchainRoutes(r)
	}
	if conf.GetGFiles() {
		m.SetGafsRoutes(r)
	}
	if conf.Config.IsSubNode() {
		m.SetSubNodeRoutes(r)
	}

	//0303
	if conf.Config.IsSupportingOBS() {
		m.SetVDESrcRoutes(r)
	}

	return r.GetAPI()
}
