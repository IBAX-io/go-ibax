/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"reflect"
)

type space string

const (
	namespaceSeparator = "."
)

const (
	namespaceRPC   space = "rpc"
	NamespaceDebug space = "debug"

	NamespaceIBAX  space = "ibax"
	NamespaceAdmin space = "admin"
	NamespaceNet   space = "net"
)

type RpcApis interface {
	GetApis() []any
}

type RpcServers struct {
	server *Server
}

type NewApiService interface {
	New(structObject any) (name string)
	Delete(name string)
}

type Method struct {
	services map[string]reflect.Value
}

func (a *Method) Delete(name string) {
	for k, _ := range a.services {
		if k == name {
			delete(a.services, k)
			break
		}
	}
}

func (a *Method) New(structObject any) (name string) {
	if a.services == nil {
		a.services = make(map[string]reflect.Value)
	}
	var findOut bool
	ref := GetStructValue(structObject)
	for _, v := range a.services {
		if v == ref {
			findOut = true
		}
	}
	name = ref.Type().String()
	if !findOut {
		a.services[name] = ref
	}
	return
}

func (r *RpcServers) Modules() ([]string, *Error) {
	r.server.service.mu.Lock()
	defer r.server.service.mu.Unlock()
	var methods []string
	for namespace, v := range r.server.service.services {
		for name := range v.callbacks {
			methods = append(methods, namespace+namespaceSeparator+name)
		}
	}
	return methods, nil
}

func GetStructValue(structObject any) reflect.Value {
	return reflect.ValueOf(structObject)
}
