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
		EcosysIDValidator:  GetEcosystemIDValidator(),
		EcosysNameGetter:   BuildEcosystemNameGetter(),
		EcosysLookupGetter: BuildEcosystemLookupGetter(),
		ContractRunner:     GetSmartContractRunner(),
		ClientTxProcessor:  GetClientTxPreprocessor(),
	}

	r := api.NewRouter(m)
	if !conf.Config.IsSupportingOBS() {
	}

	return r.GetAPI()
}
