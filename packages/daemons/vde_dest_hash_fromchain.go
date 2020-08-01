/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/IBAX-io/go-ibax/packages/converter"

	"path/filepath"

	chain_api "github.com/IBAX-io/go-ibax/packages/chain_sdk"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)

type dest_VDEDestDataHashResult struct {
	Count string `json:"count"`
	List  []struct {
		ID         string `json:"id"`
		TaskUUID   string `json:"task_uuid"`
		DataUUID   string `json:"data_uuid"`
		Hash       string `json:"hash"`
		UpdateTime string `json:"update_time"`
		CreateTime string `json:"create_time"`
	} `json:"list"`
}

//Getting task data hash from the chain
func VDEDestDataHashGetFromChain(ctx context.Context, d *daemon) error {
	var (
		blockchain_http      string
		blockchain_ecosystem string
		UpdateTime           string
		err                  error
	)

	hashtime := &model.VDEDestHashTime{}
	DestHashTime, err := hashtime.Get()
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting DestHashTime")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	if DestHashTime == nil {
		//log.Info("DestHashTime not found")
		fmt.Println("Dest DestHashTime not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}

	chaininfo := &model.VDEDestChainInfo{}
	DestChainInfo, err := chaininfo.Get()
	if err != nil {
		//log.WithFields(log.Fields{"error": err}).Error("VDE Dest fromchain getting chain info")
		log.Info("Dest chain info not found")
		time.Sleep(time.Millisecond * 100)
		return err
	}
	if DestChainInfo == nil {
		log.Info("Dest chain info not found")
		//fmt.Println("Dest chain info not found")
		time.Sleep(time.Millisecond * 100)
		return nil
	}

	blockchain_http = DestChainInfo.BlockchainHttp
	blockchain_ecosystem = DestChainInfo.BlockchainEcosystem
	//fmt.Println("DestChainInfo:", blockchain_http, blockchain_ecosystem)

	ecosystemID, err := strconv.Atoi(blockchain_ecosystem)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("encode error")
		time.Sleep(time.Millisecond * 2)
		return err
	}

	//chain_api.ApiAddress = blockchain_http
	//chain_api.ApiEcosystemID = int64(ecosystemID)

	chain_apiAddress := blockchain_http
	chain_apiEcosystemID := int64(ecosystemID)

	src := filepath.Join(conf.Config.KeysDir, "PrivateKey")
	// Login
	//if err := api.KeyLogin(src, chain_api.ApiEcosystemID); err != nil {
	//	log.WithFields(log.Fields{"error": err}).Error("Login chain failure")
	//	time.Sleep(time.Millisecond * 2)
	//	return err
	//}
	gAuth_chain, _, _, _, _, err := chain_api.KeyLogin(chain_apiAddress, src, chain_apiEcosystemID)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Login chain failure")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	//fmt.Println("Login OK!")

	create_time := converter.Int64ToStr(DestHashTime.UpdateTime)
	where_str := `{"create_time": {"$gt": ` + create_time + `}}`
	//fmt.Println("where_str:",where_str)
	form := url.Values{
		`where`: {where_str},
	}
	//var lret interface{}
	t_struct := dest_VDEDestDataHashResult{}
	url := `listWhere` + `/vde_share_hash`
	//err = api.SendPost(url, &form, &t_struct)
	//if err != nil {

	//utils.Print_json(t_struct)
	for _, DataHashItem := range t_struct.List {
		//fmt.Println("DataHashItem:", DataHashItem.ID, DataHashItem.TaskUUID)
		m := &model.VDEDestDataHash{}
		m.TaskUUID = DataHashItem.TaskUUID
		m.DataUUID = DataHashItem.DataUUID
		m.Hash = DataHashItem.Hash
		m.BlockchainHttp = blockchain_http
		m.BlockchainEcosystem = blockchain_ecosystem
		m.CreateTime = converter.StrToInt64(DataHashItem.CreateTime)

		if err = m.Create(); err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Failed to insert vde_dest_data_hash table")
			break
		}
		fmt.Println("insert vde_dest_data_hash table ok, DataUUID:", DataHashItem.DataUUID)
		UpdateTime = DataHashItem.CreateTime
	}

	DestHashTime.UpdateTime = converter.StrToInt64(UpdateTime)
	err = DestHashTime.Updates()
	if err != nil {
		fmt.Println("Update UpdateTime table err: ", err)
		log.WithFields(log.Fields{"error": err}).Error("Update UpdateTime table!")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	//fmt.Println("Update UpdateTime table OK")
	return nil
}
