/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package modes

import (
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/service/jsonrpc"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const stopTimeout = 5 * time.Second

type rpcServer struct {
	lo          sync.Mutex
	httpHandler atomic.Value
	mode        jsonrpc.Mode
}

type rpcHandler struct {
	http.Handler
	server *jsonrpc.Server
}

type adminAPI struct {
	svr *serverApi
}

func (a *adminAPI) GetApis() []any {
	var apis []any
	if a == nil {
		return nil
	}
	if a.svr != nil {
		apis = append(apis, a.svr)
	}

	return apis
}

func newAdminApi(r *rpcServer) *adminAPI {
	return &adminAPI{
		svr: newServerApi(r),
	}
}

type serverApi struct {
	rs *rpcServer
}

func newServerApi(r *rpcServer) *serverApi {
	return &serverApi{r}
}

func (s *serverApi) StartJsonRpc(ctx jsonrpc.RequestContext, namespace *string) (bool, *jsonrpc.Error) {
	if namespace != nil {
		conf.Config.JsonRPC.Namespace = *namespace
	}
	s.rs.httpHandler.Store((*rpcHandler)(nil))
	err := s.rs.enableRpc(conf.Config.JsonRPC.Namespace)
	if err != nil {
		return false, jsonrpc.DefaultError("enable rpc failed")
	}

	return true, nil
}

func (s *serverApi) StopJsonRpc() (bool, *jsonrpc.Error) {
	s.rs.lo.Lock()
	defer s.rs.lo.Unlock()
	s.rs.disableRPC()
	return true, nil
}

func (r *rpcServer) rpcIsEnable() bool {
	return r.httpHandler.Load().(*rpcHandler) != nil
}

func (r *rpcServer) getApis(namespace string) []any {
	var apis []any
	switch namespace {
	case jsonrpc.GetNamespace(jsonrpc.NamespaceAdmin):
		adminApi := newAdminApi(r)
		apis = append(apis, adminApi.GetApis()...)
	case jsonrpc.GetNamespace(jsonrpc.NamespaceIBAX):
		ibaxApi := jsonrpc.NewIbaxApi(r.mode)
		apis = append(apis, ibaxApi.GetApis()...)
	case jsonrpc.GetNamespace(jsonrpc.NamespaceNet):
		netApi := jsonrpc.NewNetApi()
		apis = append(apis, netApi.GetApis()...)
	case jsonrpc.GetNamespace(jsonrpc.NamespaceDebug):
		debugApi := jsonrpc.NewDebugApi()
		apis = append(apis, debugApi.GetApis()...)
	}
	return apis
}

func (r *rpcServer) enableRpc(namespaces string) error {
	if r.rpcIsEnable() {
		return fmt.Errorf("RPC Server is already enabled")
	}

	srv := jsonrpc.NewServer(r.mode)
	for _, m := range strings.Split(namespaces, ",") {
		name := strings.TrimSpace(m)
		funcs := r.getApis(name)
		for _, f := range funcs {
			err := srv.RegisterName(name, f)
			if err != nil {
				return err
			}
		}
	}

	r.httpHandler.Store(&rpcHandler{
		Handler: jsonrpc.NewMiddlewares(srv, r.mode),
		server:  srv,
	})
	return nil
}

func (s *rpcServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := s.httpHandler.Load().(*rpcHandler)
	if handler != nil {
		handler.ServeHTTP(w, r)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func newRpcServer(m jsonrpc.Mode) *rpcServer {
	s := &rpcServer{
		mode: m,
	}
	s.httpHandler.Store((*rpcHandler)(nil))
	return s
}

func (r *rpcServer) disableRPC() {
	handler := r.httpHandler.Load().(*rpcHandler)
	if handler != nil {
		r.httpHandler.Store((*rpcHandler)(nil))
		handler.server.Stop()
	}
}
