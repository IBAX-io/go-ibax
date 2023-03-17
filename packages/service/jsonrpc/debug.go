/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"runtime"
)

type debugApi struct {
}

func newDebugApi() *debugApi {
	return &debugApi{}
}

type memMetric struct {
	Alloc uint64 `json:"alloc"`
	Sys   uint64 `json:"sys"`
}

func (c *debugApi) GetMemStat() (*memMetric, *Error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	//Alloc: Number of bytes allocated and still in use
	//Sys: The number of bytes fetched from the system (total)
	return &memMetric{Alloc: m.Alloc, Sys: m.Sys}, nil
}

type banMetric struct {
	NodePosition int  `json:"node_position"`
	Status       bool `json:"status"`
}

func (c *debugApi) GetNodeBanStat() (*[]banMetric, *Error) {
	nodes := syspar.GetNodes()
	list := make([]banMetric, 0, len(nodes))

	b := node.GetNodesBanService()
	for i, n := range nodes {
		list = append(list, banMetric{
			NodePosition: i,
			Status:       b.IsBanned(n),
		})
	}

	return &list, nil
}
