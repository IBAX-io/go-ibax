/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	chain_api "github.com/IBAX-io/go-ibax/packages/chain_sdk"
	"github.com/IBAX-io/go-ibax/packages/crypto/ecies"

	"path/filepath"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)

//Scheduling task data hash information up the chain
func SubNodeSrcDataUpToChain(ctx context.Context, d *daemon) error {
	var (
		blockchain_table     string
		blockchain_http      string
		blockchain_ecosystem string
		err                  error
		txHash               string
	)

	m := &model.SubNodeSrcDataChainStatus{}
	SrcTaskDataChain, err := m.GetAllByChainState(0) //0 not up to chain
	if err != nil {
		time.Sleep(time.Millisecond * 200)
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data chain")
		return err
	}
	if len(SrcTaskDataChain) == 0 {
		//log.Info("Src task data chain not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}

	// deal with task data
	for _, item := range SrcTaskDataChain {
		//fmt.Println("TaskUUID:", item.TaskUUID)
		blockchain_table = item.BlockchainTable
		blockchain_http = item.BlockchainHttp
		blockchain_ecosystem = item.BlockchainEcosystem
		fmt.Println("blockchain_http:", blockchain_http, blockchain_ecosystem, blockchain_table)

		ecosystemID, err := strconv.Atoi(blockchain_ecosystem)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("encode error")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		chain_apiAddress := blockchain_http
		chain_apiEcosystemID := int64(ecosystemID)

		src := filepath.Join(conf.Config.KeysDir, "chain_PrivateKey")
		// Login
		gAuth_chain, _, gPrivate_chain, _, _, err := chain_api.KeyLogin(chain_apiAddress, src, chain_apiEcosystemID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Login chain failure")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		fmt.Println("Login chain OK!")

		if item.TranMode == 1 { // hash uptochain
			form := url.Values{
				`TableName`: {blockchain_table},
				`TaskUUID`:  {item.TaskUUID},
				`DataUUID`:  {item.DataUUID},
				`DataInfo`:  {item.DataInfo},
				`Data`:      {item.Hash},
			}

			tran_mode := converter.Int64ToStr(item.TranMode)
			chain_api.ApiPrivateFor = []string{
				tran_mode,
				//"1",
				//node_pubkey,
			}
			node_pubkey_slice := strings.Split(item.SubNodeDestPubkey, ";")
			chain_api.ApiPrivateFor = append(chain_api.ApiPrivateFor, node_pubkey_slice...)

			ContractName := `@1SubNodeSrcHashCreate`
			_, txHash, _, err = chain_api.VDEPostTxResult(chain_apiAddress, chain_apiEcosystemID, gAuth_chain, gPrivate_chain, ContractName, &form)
			if err != nil {
				fmt.Println("Send SubNodeSrcData to chain err: ", err)
				log.WithFields(log.Fields{"error": err}).Error("Send SubNodeSrcData to chain!")
				time.Sleep(time.Second * 5)
				continue
			}
			fmt.Println("Send chain Contract to run, ContractName:", ContractName)

			item.ChainState = 1 //success
			item.TxHash = txHash
			item.BlockId = 0
			item.ChainErr = ""
		} else if item.TranMode == 2 { // all data uptochain
			node_pubkey_slice := strings.Split(item.SubNodeDestPubkey, ";")
			for _, pubkey_value := range node_pubkey_slice {
				PrivateFile, err := ecies.EccCryptoKey(item.Data, pubkey_value)
				if err != nil {
					fmt.Println("error", err)
					log.WithFields(log.Fields{"error": err}).Error("EccCryptoKey error")
					return nil
				}
				encodeString := base64.StdEncoding.EncodeToString(PrivateFile)

				form := url.Values{
					`TableName`: {blockchain_table},
					`TaskUUID`:  {item.TaskUUID},
					`DataUUID`:  {item.DataUUID},
					`DataInfo`:  {item.DataInfo},
					`Data`:      {encodeString},
				}
				tran_mode := converter.Int64ToStr(item.TranMode)
				chain_api.ApiPrivateFor = []string{
					tran_mode,
					//"1",
					//node_pubkey,
				}
				chain_api.ApiPrivateFor = append(chain_api.ApiPrivateFor, pubkey_value)

				ContractName := `@1SubNodeSrcDataCreate`
				_, txHash, _, err = chain_api.VDEPostTxResult(chain_apiAddress, chain_apiEcosystemID, gAuth_chain, gPrivate_chain, ContractName, &form)
				if err != nil {
					fmt.Println("Send SubNodeSrcData to chain err: ", err)
					log.WithFields(log.Fields{"error": err}).Error("Send SubNodeSrcData to chain!")
					time.Sleep(time.Second * 5)
					continue
			item.ChainErr = "TranMode err!"
		}

		//item.ChainState = 1
		//item.TxHash = txHash
		//item.BlockId = 0
		//item.ChainErr = ""
		item.UpdateTime = time.Now().Unix()
		err = item.Updates()
		if err != nil {
			fmt.Println("Update SubNodeSrcDataChainStatus table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update SubNodeSrcDataChainStatus table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}
	} //for
	return nil
}

//Query the status of the chain on the scheduling task data hash information
func SubNodeSrcHashUpToChainState(ctx context.Context, d *daemon) error {
	var (
		blockchain_http      string
		blockchain_ecosystem string
		err                  error
	)

	m := &model.SubNodeSrcDataChainStatus{}
	SrcTaskDataChain, err := m.GetAllByChainState(1) //1up to chain
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data chain")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	if len(SrcTaskDataChain) == 0 {
		//log.Info("Src task data chain not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}

	// deal with task data
	for _, item := range SrcTaskDataChain {
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

		src := filepath.Join(conf.Config.KeysDir, "chain_PrivateKey")
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
			time.Sleep(2 * time.Second)
			continue
		}
		err = item.Updates()
		if err != nil {
			fmt.Println("Update SubNodeSrcDataChainStatus table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update SubNodeSrcDataChainStatus table!")
			time.Sleep(2 * time.Second)
			continue
		}
		fmt.Println("SubNode SrcData Run chain Contract ok, TxHash:", string(item.TxHash))
		time.Sleep(time.Millisecond * 200)
	} //for
	return nil
}
