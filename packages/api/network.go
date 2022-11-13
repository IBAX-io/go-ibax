/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"
	"strconv"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/converter"
)

type HonorNodeJSON struct {
	TCPAddress string `json:"tcp_address"`
	APIAddress string `json:"api_address"`
	PublicKey  string `json:"public_key"`
	UnbanTime  string `json:"unban_time"`
	Stopped    bool   `json:"stopped"`
}

type NetworkResult struct {
	NetworkID     string          `json:"network_id"`
	CentrifugoURL string          `json:"centrifugo_url"`
	Test          bool            `json:"test"`
	Private       bool            `json:"private"`
	HonorNodes    []HonorNodeJSON `json:"honor_nodes"`
}

func GetNodesJSON() []HonorNodeJSON {
	nodes := make([]HonorNodeJSON, 0)
	for _, node := range syspar.GetNodes() {
		nodes = append(nodes, HonorNodeJSON{
			TCPAddress: node.TCPAddress,
			APIAddress: node.APIAddress,
			PublicKey:  crypto.PubToHex(node.PublicKey),
			UnbanTime:  strconv.FormatInt(node.UnbanTime.Unix(), 10),
		})
	}
	return nodes
}

func getNetworkHandler(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, &NetworkResult{
		NetworkID:     converter.Int64ToStr(conf.Config.LocalConf.NetworkID),
		CentrifugoURL: conf.Config.Centrifugo.URL,
		Private:       syspar.IsPrivateBlockchain(),
		HonorNodes:    GetNodesJSON(),
	})
}
