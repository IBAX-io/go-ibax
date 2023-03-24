/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package modes

import (
	"context"
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/service/jsonrpc"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const stopTimeout = 5 * time.Second

type rpcServer struct {
	lo          sync.Mutex
	server      *http.Server
	httpHandler atomic.Value
	mode        jsonrpc.Mode
	listener    net.Listener
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

func (s *serverApi) StartJsonRpc(ctx jsonrpc.RequestContext, host *string, port *int, namespace *string) (bool, *jsonrpc.Error) {
	if host == nil {
		h := conf.Config.JsonRPC.Host
		host = &h
	}
	if port == nil {
		p := conf.Config.JsonRPC.Port
		port = &p
	}

	if namespace != nil {
		conf.Config.JsonRPC.Namespace = *namespace
	}

	addr := fmt.Sprintf("%s:%d", *host, *port)
	if s.rs.listener != nil {
		addrA := addr
		addrB := s.rs.listener.Addr().String()
		hostA := strings.Split(addrA, ":")
		hostB := strings.Split(addrB, ":")
		if addrA == addrB || (hostA[1] == hostB[1] && isLocalhost(hostA[2], hostB[2])) {
			return false, jsonrpc.DefaultError(fmt.Sprintf("HTTP server already running on %s", s.rs.listener.Addr().String()))
		}
	}
	err := startRPC(addr, s.rs.mode)
	if err != nil {
		return false, jsonrpc.DefaultError(err.Error())
	}
	w := ctx.HTTPResponseWriter()
	_, _ = w.Write([]byte("true"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	time.Sleep(2 * time.Second)
	_, jErr := s.StopJsonRpc()
	if jErr != nil {
		return false, jErr
	}

	return true, nil
}

func (s *serverApi) StopJsonRpc() (bool, *jsonrpc.Error) {
	if s.rs.listener == nil {
		return true, nil
	}
	s.rs.disableRPC()

	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	err := s.rs.server.Shutdown(ctx)
	if err != nil && err == ctx.Err() {
		log.Warn("HTTP server graceful shutdown timed out")
		s.rs.server.Close()
	}

	s.rs.listener.Close()
	log.Info("HTTP server stopped", "endpoint", s.rs.listener.Addr())

	s.rs.server, s.rs.listener = nil, nil
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
		for _, v := range adminApi.GetApis() {
			apis = append(apis, v)
		}
	case jsonrpc.GetNamespace(jsonrpc.NamespaceIBAX):
		ibaxApi := jsonrpc.NewIbaxApi(r.mode)
		for _, v := range ibaxApi.GetApis() {
			apis = append(apis, v)
		}
	case jsonrpc.GetNamespace(jsonrpc.NamespaceNet):
		netApi := jsonrpc.NewNetApi()
		for _, v := range netApi.GetApis() {
			apis = append(apis, v)
		}
	case jsonrpc.GetNamespace(jsonrpc.NamespaceDebug):
		debugApi := jsonrpc.NewDebugApi()
		for _, v := range debugApi.GetApis() {
			apis = append(apis, v)
		}
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
		if funcs != nil {
			for _, f := range funcs {
				err := srv.RegisterName(name, f)
				if err != nil {
					return err
				}
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

func startRPC(addr string, m jsonrpc.Mode) error {
	if !conf.Config.JsonRPC.Enabled {
		return nil
	}
	rpc := newRpcServer(m)
	return rpc.start(addr)
}

func (r *rpcServer) start(addr string) error {
	r.lo.Lock()
	defer r.lo.Unlock()
	err := r.enableRpc(conf.Config.JsonRPC.Namespace)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		r.disableRPC()
		return err
	}
	r.listener = listener

	server := &http.Server{
		Handler:           r,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	r.server = server
	if conf.Config.TLSConf.Enabled {
		if len(conf.Config.TLSConf.TLSCert) == 0 || len(conf.Config.TLSConf.TLSKey) == 0 {
			log.Fatal("[JSON-RPC] -tls-cert/TLSCert and -tls-key/TLSKey must be specified with -tls/TLS")
		}
		if _, err := os.Stat(conf.Config.TLSConf.TLSCert); os.IsNotExist(err) {
			log.WithError(err).Fatalf(`[JSON-RPC] Filepath -tls-cert/TLSCert = %s is invalid`, conf.Config.TLSConf.TLSCert)
		}
		if _, err := os.Stat(conf.Config.TLSConf.TLSKey); os.IsNotExist(err) {
			log.WithError(err).Fatalf(`[JSON-RPC] Filepath -tls-key/TLSKey = %s is invalid`, conf.Config.TLSConf.TLSKey)
		}
		go func() {
			err := server.ListenAndServeTLS(conf.Config.TLSConf.TLSCert, conf.Config.TLSConf.TLSKey)
			if err != nil {
				log.WithFields(log.Fields{"host": addr, "error": err, "type": consts.NetworkError}).Fatal("[JSON-RPC] Listening TLS server")
			}
		}()
		log.WithFields(log.Fields{"host": addr}).Info("[JSON-RPC] listening with TLS at")
		return nil
	}
	go func() {
		err := server.Serve(listener)
		if err != nil {
			log.WithFields(log.Fields{"host": addr, "error": err, "type": consts.NetworkError}).Fatal("[JSON-RPC] Listening server")
		}
	}()
	return nil
}

func isLocalhost(hostA, hostB string) bool {
	if (strings.Contains(hostA, "localhost") || strings.Contains(hostA, "127.0.0.1")) &&
		(strings.Contains(hostB, "localhost") || strings.Contains(hostB, "127.0.0.1")) {
		return true
	}
	return false
}
