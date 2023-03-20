/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/service/node"
)

type NetApi struct {
	net *networkApi
}

func NewNetApi() *NetApi {
	return &NetApi{
		net: NewNetworkApi(),
	}
}

func (p *NetApi) GetApis() []any {
	var apis []any
	if p == nil {
		return nil
	}
	if p.net != nil {
		apis = append(apis, p.net)
	}
	return apis
}

type networkApi struct {
}

func NewNetworkApi() *networkApi {
	n := &networkApi{}
	return n
}

func (n *networkApi) Status() string {
	return node.NodePauseType().String()
}

func (c *networkApi) GetNetwork() (*NetworkResult, *Error) {
	return &NetworkResult{
		NetworkID:     converter.Int64ToStr(conf.Config.LocalConf.NetworkID),
		CentrifugoURL: conf.Config.Centrifugo.URL,
		Test:          syspar.IsTestMode(),
		Private:       syspar.IsPrivateBlockchain(),
		HonorNodes:    getNodesJSON(),
	}, nil
}
