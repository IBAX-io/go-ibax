/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package modes

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/api"
	"github.com/IBAX-io/go-ibax/packages/conf"
)

func RegisterRoutes() http.Handler {
	m := api.Mode{
		EcosystemGetter:   GetEcosystemGetter(),
		ContractRunner:    GetSmartContractRunner(),
		ClientTxProcessor: GetClientTxPreprocessor(),
	}

	r := api.NewRouter(m)
	if !conf.Config.IsSupportingCLB() {
		m.SetBlockchainRoutes(r)
	}

	if conf.Config.IsSubNode() {
		m.SetSubNodeRoutes(r)
	}

	if conf.Config.IsSupportingCLB() {
	}

	return r.GetAPI()
}
