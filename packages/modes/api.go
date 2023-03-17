/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package modes

import (
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/service/jsonrpc"
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

func RegisterJsonRPC(host string) {
	m := jsonrpc.Mode{
		EcosystemGetter:   GetEcosystemGetter(),
		ContractRunner:    GetSmartContractRunner(),
		ClientTxProcessor: GetClientTxPreprocessor(),
	}
	err := startRPC(host, m)
	if err != nil {
		panic(fmt.Sprintf("start RPC failed:%s", err.Error()))
	}
}
