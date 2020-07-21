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

	chain_api "github.com/IBAX-io/go-ibax/packages/chain_sdk"

	"path/filepath"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)

//Scheduling task information up the chain
func VDESrcTaskUpToChain(ctx context.Context, d *daemon) error {
	var (
		blockchain_http      string
		blockchain_ecosystem string
		err                  error
	)

	m := &model.VDESrcTaskChainStatus{}
	SrcTask, err := m.GetAllByContractStateAndChainState(1, 0, 0) //0
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	if len(SrcTask) == 0 {
		//log.Info("Src task not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}

	chaininfo := &model.VDESrcChainInfo{}
	SrcChainInfo, err := chaininfo.Get()
	if err != nil {
		//log.WithFields(log.Fields{"error": err}).Error("VDE Src uptochain getting chain info")
		log.Info("Src chain info not found")
		time.Sleep(time.Millisecond * 100)
		return err
	}
	if SrcChainInfo == nil {
		log.Info("Src chain info not found")
		//fmt.Println("Src chain info not found")
		time.Sleep(time.Millisecond * 100)
		return nil
	}

	blockchain_http = SrcChainInfo.BlockchainHttp
	blockchain_ecosystem = SrcChainInfo.BlockchainEcosystem
	//fmt.Println("SrcChainInfo:", blockchain_http, blockchain_ecosystem)
	// deal with task data
	for _, item := range SrcTask {
		//fmt.Println("SrcTask:", item.TaskUUID)

		ecosystemID, err := strconv.Atoi(blockchain_ecosystem)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("encode error")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		chain_apiAddress := blockchain_http
		chain_apiEcosystemID := int64(ecosystemID)

		src := filepath.Join(conf.Config.KeysDir, "PrivateKey")
		// Login
		gAuth_chain, _, gPrivate_chain, _, _, err := chain_api.KeyLogin(chain_apiAddress, src, chain_apiEcosystemID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Login chain failure")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		//fmt.Println("Login OK!")

		form := url.Values{
			"TaskUUID":     {item.TaskUUID},
			"TaskName":     {item.TaskName},
			"TaskSender":   {item.TaskSender},
			"Comment":      {item.Comment},
			"Parms":        {item.Parms},
			"TaskType":     {converter.Int64ToStr(item.TaskType)},
			"TaskState":    {converter.Int64ToStr(item.TaskState)},

			"ContractSrcName":     {item.ContractSrcName},
			"ContractSrcGet":      {item.ContractSrcGet},
			"ContractSrcGetHash":  {item.ContractSrcGetHash},
			"ContractDestName":    {item.ContractDestName},
			"ContractDestGet":     {item.ContractDestGet},
			"ContractDestGetHash": {item.ContractDestGetHash},

			"ContractRunHttp":      {item.ContractRunHttp},
			"ContractRunEcosystem": {item.ContractRunEcosystem},
			"ContractRunParms":     {item.ContractRunParms},

			"ContractMode": {converter.Int64ToStr(item.ContractMode)},
			`CreateTime`:   {converter.Int64ToStr(time.Now().Unix())},
		}

		ContractName := `@1VDEShareTaskCreate`
		_, txHash, _, err := chain_api.VDEPostTxResult(chain_apiAddress, chain_apiEcosystemID, gAuth_chain, gPrivate_chain, ContractName, &form)
		if err != nil {
			fmt.Println("Send VDESrcTask to chain err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Send VDESrcTask to chain!")
			time.Sleep(time.Second * 5)
			continue
		}
		fmt.Println("Send chain Contract to run, ContractName:", ContractName)

		item.ChainState = 1
		item.TxHash = txHash
		item.BlockId = 0
		item.ChainErr = ""
		item.UpdateTime = time.Now().Unix()
		err = item.Updates()
		if err != nil {
			fmt.Println("Update VDESrcTask table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTask table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}
	} //for
	return nil
}

//Query the status of the chain on the scheduling task information
func VDESrcTaskUpToChainState(ctx context.Context, d *daemon) error {
	var (
		//TaskParms      map[string]interface{}

		blockchain_http      string
		blockchain_ecosystem string

		//ok        bool
		err error
	)

	m := &model.VDESrcTaskChainStatus{}
	SrcTask, err := m.GetAllByContractStateAndChainState(1, 0, 1) //
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	if len(SrcTask) == 0 {
		//log.Info("Src task not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}
	chaininfo := &model.VDESrcChainInfo{}
	SrcChainInfo, err := chaininfo.Get()
	if err != nil {
		//log.WithFields(log.Fields{"error": err}).Error("VDE Src uptochain getting chain info")
		log.Info("Src chain info not found")
		time.Sleep(time.Millisecond * 100)
		return err
	}
	if SrcChainInfo == nil {
		log.Info("Src chain info not found")
		//fmt.Println("Src chain info not found")
		time.Sleep(time.Millisecond * 100)
		return nil
	}

	blockchain_http = SrcChainInfo.BlockchainHttp
	blockchain_ecosystem = SrcChainInfo.BlockchainEcosystem
	//fmt.Println("SrcChainInfo:", blockchain_http, blockchain_ecosystem)

	// deal with task data
	for _, item := range SrcTask {
		//fmt.Println("SrcTask:", item.TaskUUID)

		ecosystemID, err := strconv.Atoi(blockchain_ecosystem)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("encode error")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		chain_apiAddress := blockchain_http
		chain_apiEcosystemID := int64(ecosystemID)

		src := filepath.Join(conf.Config.KeysDir, "PrivateKey")
		// Login
		gAuth_chain, _, _, _, _, err := chain_api.KeyLogin(chain_apiAddress, src, chain_apiEcosystemID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Login chain failure")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		//fmt.Println("Login OK!")

		blockId, err := chain_api.VDEWaitTx(chain_apiAddress, gAuth_chain, string(item.TxHash))
		if blockId > 0 {
			item.BlockId = blockId
			item.ChainId = converter.StrToInt64(err.Error())
			item.ChainState = 2
			item.ChainErr = ""

		} else if blockId == 0 {
			//item.ChainState = 3
			item.ChainState = 1 //
			item.ChainErr = err.Error()
		} else {
			//fmt.Println("VDEWaitTx! err: ", err)
			time.Sleep(time.Millisecond * 2)
			continue
		}
		err = item.Updates()
		if err != nil {
			fmt.Println("Update VDESrcTask table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTask table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		//fmt.Println("Run chain Contract ok, TxHash:", string(item.TxHash))
	} //for
	return nil
}
