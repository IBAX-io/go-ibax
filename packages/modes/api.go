/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package modes

import (
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

type JsonRpcRoutes struct {
	s    *rpcServer
	next http.Handler
}

func (s *JsonRpcRoutes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "/" {
		s.s.ServeHTTP(w, r)
		return
	}
	s.next.ServeHTTP(w, r)
}

func RegisterJsonRPCRoutes(next http.Handler) http.Handler {
	m := jsonrpc.Mode{
		EcosystemGetter:   GetEcosystemGetter(),
		ContractRunner:    GetSmartContractRunner(),
		ClientTxProcessor: GetClientTxPreprocessor(),
	}

	rpc := newRpcServer(m)
	rpc.lo.Lock()
	defer rpc.lo.Unlock()
	err := rpc.enableRpc(conf.Config.JsonRPC.Namespace)
	if err != nil {
		panic(err)
	}
	return &JsonRpcRoutes{rpc, next}
}
