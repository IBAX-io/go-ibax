/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"
	"sync/atomic"
)

type Server struct {
	service serviceRegistry
	status  int32
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Permit dumb empty requests for remote health-checks (AWS)
	if r.Method == http.MethodGet && r.ContentLength == 0 && r.URL.RawQuery == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if code, err := validateRequest(r); err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	if atomic.LoadInt32(&s.status) == 0 {
		return
	}

	s.service.ServeHTTP(w, r)
}

func NewServer(m Mode) *Server {
	server := &Server{
		status: 1,
	}
	server.service.mode = m

	rpcService := &RpcServers{server}

	name := GetNamespace(namespaceRPC)
	err := server.RegisterName(name, rpcService)
	if err != nil {
		panic(fmt.Sprintf("register name[%s] failed:%s\n", name, err.Error()))
	}

	return server
}

func (s *Server) RegisterName(namespace string, function any) error {
	return s.service.registerName(namespace, function)
}

func (s *Server) Stop() {
	s.service.mu.Lock()
	defer s.service.mu.Unlock()

	if atomic.CompareAndSwapInt32(&s.status, 1, 0) {
		log.Debug("Json-RPC server shutting down")
	}
}
