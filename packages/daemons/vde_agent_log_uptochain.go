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

//Scheduling task data log information up the chain
func VDEAgentLogUpToChain(ctx context.Context, d *daemon) error {
	var (
		blockchain_http      string
		blockchain_ecosystem string
		err                  error
	)

	m := &model.VDEAgentDataLog{}
	AgentTaskDataLog, err := m.GetAllByChainState(0) // 0 not up to chain
	if err != nil {
		time.Sleep(time.Millisecond * 2)
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data log")
		return err
	}
	if len(AgentTaskDataLog) == 0 {
		//log.Info("Src task data log not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}

	//chaininfo := &model.VDESrcChainInfo{}
	//SrcChainInfo, err := chaininfo.Get()
	//if err != nil {
	//	log.WithFields(log.Fields{"error": err}).Error("VDE Src uptochain getting chain info")
	//	time.Sleep(time.Second * 30)
	//	return err
	//}
	//if SrcChainInfo == nil {
	//	//log.Info("Src chain info not found")
	//	fmt.Println("Src chain info not found")
	//	time.Sleep(time.Second * 2)
	//	return nil
	//}

	// deal with task data
	for _, item := range AgentTaskDataLog {
		//fmt.Println("TaskUUID:", item.TaskUUID)
		blockchain_http = item.BlockchainHttp
		blockchain_ecosystem = item.BlockchainEcosystem
		//fmt.Println("blockchain_http:", blockchain_http, blockchain_ecosystem)

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
			"TaskUUID":  {item.TaskUUID},
			"DataUUID":  {item.DataUUID},
			"Log":       {item.Log},
			"LogType":   {converter.Int64ToStr(item.LogType)},
			"LogSender": {item.LogSender},

			`CreateTime`: {converter.Int64ToStr(time.Now().Unix())},
		}

		ContractName := `@1VDEShareLogCreate`
		_, txHash, _, err := chain_api.VDEPostTxResult(chain_apiAddress, chain_apiEcosystemID, gAuth_chain, gPrivate_chain, ContractName, &form)
		if err != nil {
			fmt.Println("Send VDEAgentLog to chain err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Send VDEAgentLog to chain!")
			time.Sleep(time.Second * 5)
			continue
		item.ChainState = 1
		item.TxHash = txHash
		item.BlockId = 0
		item.ChainErr = ""
		item.UpdateTime = time.Now().Unix()
		err = item.Updates()
		if err != nil {
			fmt.Println("Update VDESrcLog table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDESrcLog table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}
	}

	return nil
}

//Query the status of the chain on the scheduling task data log information
func VDEAgentLogUpToChainState(ctx context.Context, d *daemon) error {
	var (
		blockchain_http      string
		blockchain_ecosystem string
		err                  error
	)

	m := &model.VDEAgentDataLog{}
	AgentTaskDataLog, err := m.GetAllByChainState(1) //1 up to chain
	if err != nil {
		time.Sleep(time.Millisecond * 2)
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data log")
		return err
	}
	if len(AgentTaskDataLog) == 0 {
		//log.Info("Agent task data log not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}
	//chaininfo := &model.VDESrcChainInfo{}
	//SrcChainInfo, err := chaininfo.Get()
	//if err != nil {
	//	log.WithFields(log.Fields{"error": err}).Error("VDE Src uptochain getting chain info")
	//	time.Sleep(time.Second * 30)
	//	return err
	//}
	//if SrcChainInfo == nil {
	//	//log.Info("Src chain info not found")
	//	fmt.Println("Src chain info not found")
	//	time.Sleep(time.Second * 2)
	//	return nil
	//}

	// deal with task data
	for _, item := range AgentTaskDataLog {
		//fmt.Println("TaskUUID:", item.TaskUUID)
		blockchain_http = item.BlockchainHttp
		blockchain_ecosystem = item.BlockchainEcosystem
		//fmt.Println("blockchain_http:", blockchain_http, blockchain_ecosystem)
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
			item.ChainState = 1 //find check
			item.ChainErr = err.Error()
		} else {
			//fmt.Println("VDEWaitTx! err: ", err)
			time.Sleep(time.Millisecond * 2)
			continue
		}
		err = item.Updates()
		if err != nil {
			fmt.Println("Update VDEAgentLog table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDEAgentLog table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		//fmt.Println("Run chain Contract ok, TxHash:", string(item.TxHash))
	} //for

	return nil
}
