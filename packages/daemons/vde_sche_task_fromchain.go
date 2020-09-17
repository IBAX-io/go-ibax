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
	"time"

	chain_api "github.com/IBAX-io/go-ibax/packages/chain_sdk"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/crypto/ecies"

	"path/filepath"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/crypto"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

type sche_VDEShareTaskResult struct {
	Count string `json:"count"`
	List  []struct {
		ID                   string `json:"id"`
		TaskUUID             string `json:"task_uuid"`
		TaskName             string `json:"task_name"`
		TaskSender           string `json:"task_sender"`
		TaskReceiver         string `json:"task_receiver"`
		Comment              string `json:"comment"`
		Parms                string `json:"parms"`
		TaskType             string `json:"task_type"`
		TaskState            string `json:"task_state"`
		ContractSrcName      string `json:"contract_src_name"`
		ContractSrcGet       string `json:"contract_src_get"`
		ContractSrcGetHash   string `json:"contract_src_get_hash"`
		ContractDestName     string `json:"contract_dest_name"`
		ContractDestGet      string `json:"contract_dest_get"`
		ContractDestGetHash  string `json:"contract_dest_get_hash"`
		ContractRunHttp      string `json:"contract_run_http"`
		ContractRunEcosystem string `json:"contract_run_ecosystem"`
		ContractRunParms     string `json:"contract_run_parms"`
		ContractMode         string `json:"contract_mode"`
		ContractState        string `json:"contract_state"`
		UpdateTime           string `json:"update_time"`
		CreateTime           string `json:"create_time"`
		Deleted              string `json:"deleted"`
		DateDeleted          string `json:"date_deleted"`
	} `json:"list"`
}

//Getting task information from the chain
func VDEScheTaskSrcGetFromChain(ctx context.Context, d *daemon) error {

	// not open
	time.Sleep(time.Millisecond * 1000 * 20)
	return nil

	var (
		blockchain_http      string
		blockchain_ecosystem string
		SrcUpdateTime        string
		err                  error

		myContractSrcGet      string
		myContractSrcGetHash  string
		myContractDestGet     string
		myContractDestGetHash string

		ContractSrcGetHashHex  string
		ContractDestGetHashHex string
	)

	tasktime := &model.VDEScheTaskTime{}
	ScheTaskTime, err := tasktime.Get()
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting ScheTaskTime")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	if ScheTaskTime == nil {
		//log.Info("DestTaskTime not found")
		fmt.Println("Sche ScheTaskTime not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}

	chaininfo := &model.VDEScheChainInfo{}
	ScheChainInfo, err := chaininfo.Get()
	if err != nil {
		//log.WithFields(log.Fields{"error": err}).Error("VDE Sche fromchain getting chain info")
		log.Info("Sche chain info not found")
		time.Sleep(time.Millisecond * 100)
		return err
	}
	if ScheChainInfo == nil {
		log.Info("Sche chain info not found")
		//fmt.Println("Sche chain info not found")
		time.Sleep(time.Millisecond * 100)
		return nil
	}

	blockchain_http = ScheChainInfo.BlockchainHttp
	blockchain_ecosystem = ScheChainInfo.BlockchainEcosystem
	//fmt.Println("ScheChainInfo:", blockchain_http, blockchain_ecosystem)

	ecosystemID, err := strconv.Atoi(blockchain_ecosystem)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("encode error")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	//api.ApiAddress = blockchain_http
	//api.ApiEcosystemID = int64(ecosystemID)
	chain_apiAddress := blockchain_http
	chain_apiEcosystemID := int64(ecosystemID)

	src := filepath.Join(conf.Config.KeysDir, "PrivateKey")
	// Login
	//err := api.KeyLogin(src, api.ApiEcosystemID)
	gAuth_chain, _, _, _, _, err := chain_api.KeyLogin(chain_apiAddress, src, chain_apiEcosystemID)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Login chain failure")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	//fmt.Println("Login OK!")
	_, NodePublicKey, err := utils.VDEGetNodeKeys()
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("GetNodeKeys")
		return err
	}
	nodePrivateKey, err := utils.GetNodePrivateKey()
	if err != nil || len(nodePrivateKey) < 1 {
		if err == nil {
			log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node private key is empty")
		}
		return err
	}
	create_time := converter.Int64ToStr(ScheTaskTime.SrcUpdateTime)
	//where_str := `{"create_time": {"$gt": ` + create_time + `},` + ` "contract_mode": "0"}` //0
	//where_str := `{"create_time": {"$gt": ` + create_time + `},` + ` "task_receiver": ` + NodePublicKey + `, "contract_mode": "0"}`
	//where_str := `{"create_time": {"$gt": ` + create_time + `},` + ` "task_receiver": ` + NodePublicKey + `, "contract_mode": {"$in": [0,2]}}`
	where_str := `{"create_time": {"$gt": ` + create_time + `},` + ` "task_receiver": ` + NodePublicKey + `, "contract_mode": {"$in": [1,3]}}`
	//fmt.Println("where_str:",where_str)
	form := url.Values{
		`where`: {where_str},
	}
	//var lret interface{}
	t_struct := sche_VDEShareTaskResult{}
	url := `listWhere` + `/vde_share_task`

	//err = api.SendPost(url, &form, &t_struct)
	err = chain_api.SendPost(chain_apiAddress, gAuth_chain, url, &form, &t_struct)
	if err != nil {
		fmt.Println("error", err)
		return err
	}
	if len(t_struct.List) == 0 {
		log.Info("ShareTaskItem not found, sleep...")
		//fmt.Println("ShareTaskItem not found, sleep...")
		time.Sleep(time.Second * 2)
		return nil
	}

	//utils.Print_json(t_struct)
	for _, ShareTaskItem := range t_struct.List {
		fmt.Println("NodeKey:", NodePublicKey)
		fmt.Println("ShareTaskItem ID,TaskUUID,TaskReceiver:", ShareTaskItem.ID, ShareTaskItem.TaskUUID, ShareTaskItem.TaskReceiver)
		//
		//fmt.Println(":", ShareTaskItem.ContractSrcGet)
		//if ShareTaskItem.ContractMode == "2" || ShareTaskItem.ContractMode == "3" {
		if ShareTaskItem.ContractMode == "3" || ShareTaskItem.ContractMode == "4" {
			contractSrcDataBase64, err := base64.StdEncoding.DecodeString(ShareTaskItem.ContractSrcGet)
			if err != nil {
				log.WithFields(log.Fields{"error": err}).Error("base64 DecodeString err")
				fmt.Println("base64 DecodeString err")
				continue
			}
			//fmt.Println("base64:", contractSrcDataBase64)

			ContractSrcGetPlusHash, err := ecies.EccDeCrypto(contractSrcDataBase64, nodePrivateKey)
			if err != nil {
				fmt.Println("Decryption error:", err)
				log.WithFields(log.Fields{"type": consts.CryptoError}).Error("Decryption error")
				continue
			}

			//fmt.Println(":", ContractSrcGetPlusHash)

			myContractSrcGetHash = string(ContractSrcGetPlusHash)[:64]
			myContractSrcGet = string(ContractSrcGetPlusHash)[64:]
			//fmt.Println("myContractSrcGetHash", myContractSrcGetHash)
			//fmt.Println("myContractSrcGet", myContractSrcGet)

			ShareTaskItem.ContractSrcGet = myContractSrcGet
			ShareTaskItem.ContractSrcGetHash = myContractSrcGetHash

			contractDestDataBase64, err := base64.StdEncoding.DecodeString(ShareTaskItem.ContractDestGet)
			if err != nil {
				log.WithFields(log.Fields{"error": err}).Error("base64 DecodeString err")
				fmt.Println("base64 DecodeString err")
				continue
			}
			ContractDestGetPlusHash, err := ecies.EccDeCrypto(contractDestDataBase64, nodePrivateKey)
			if err != nil {
				fmt.Println("Decryption error:", err)
				log.WithFields(log.Fields{"type": consts.CryptoError}).Error("Decryption error")
				continue
			}
			//fmt.Println(":", ContractDestGetPlusHash)
			myContractDestGetHash = string(ContractDestGetPlusHash)[:64]
			myContractDestGet = string(ContractDestGetPlusHash)[64:]
			//fmt.Println("myContractDestGetHash:", myContractDestGetHash)
			//fmt.Println("myContractDestGet:", myContractDestGet)

			ShareTaskItem.ContractDestGet = myContractDestGet
			ShareTaskItem.ContractDestGetHash = myContractDestGetHash

		}

		//
		if ContractSrcGetHashHex, err = crypto.HashHex([]byte(ShareTaskItem.ContractSrcGet)); err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Raw data hash failed")
			fmt.Println("ContractSrcGetHashHex Raw data hash failed ")
			continue
		}
		if ContractSrcGetHashHex != ShareTaskItem.ContractSrcGetHash {
			log.WithFields(log.Fields{"error": err}).Error("Contract Src Hash validity fails")
			fmt.Println("Contract Src Hash validity fails")
			continue
		}
		if ContractDestGetHashHex, err = crypto.HashHex([]byte(ShareTaskItem.ContractDestGet)); err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Raw data hash failed")
			fmt.Println("ContractDestGetHashHex Raw data hash failed ")
			continue
		}
		if ContractDestGetHashHex != ShareTaskItem.ContractDestGetHash {
			log.WithFields(log.Fields{"error": err}).Error("Contract Dest Hash validity fails")
			fmt.Println("Contract Dest Hash validity fails")
			continue
		}

		m := &model.VDEScheTaskFromSrc{}
		m.TaskUUID = ShareTaskItem.TaskUUID
		m.TaskName = ShareTaskItem.TaskName
		m.TaskSender = ShareTaskItem.TaskSender
		m.Comment = ShareTaskItem.Comment
		m.Parms = ShareTaskItem.Parms
		m.TaskType = converter.StrToInt64(ShareTaskItem.TaskType)
		m.TaskState = converter.StrToInt64(ShareTaskItem.TaskState)
		m.ContractSrcName = ShareTaskItem.ContractSrcName
		m.ContractSrcGet = ShareTaskItem.ContractSrcGet
		m.ContractSrcGetHash = ShareTaskItem.ContractSrcGetHash
		m.ContractDestName = ShareTaskItem.ContractDestName
		m.ContractDestGet = ShareTaskItem.ContractDestGet
		m.ContractDestGetHash = ShareTaskItem.ContractDestGetHash

		m.ContractRunHttp = ShareTaskItem.ContractRunHttp
		m.ContractRunEcosystem = ShareTaskItem.ContractRunEcosystem
		m.ContractRunParms = ShareTaskItem.ContractRunParms

		m.ContractMode = converter.StrToInt64(ShareTaskItem.ContractMode)
		//m.ContractState = converter.StrToInt64(ShareTaskItem.ContractState)

		m.CreateTime = time.Now().Unix()
		//If the record exists, update it. to do...
		update_flag := 1
		m2 := &model.VDEScheTaskFromSrc{}
		myTask, err := m2.GetOneByTaskUUID(ShareTaskItem.TaskUUID)
		if err != nil {
			update_flag = 0
		}
		//
		if update_flag == 1 {
			m.ID = myTask.ID
			if err = m.Updates(); err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Failed to update table")
			}
		} else {
			if err = m.Create(); err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Failed to insert table")
			}
		}
		SrcUpdateTime = ShareTaskItem.CreateTime
	}

	ScheTaskTime.SrcUpdateTime = converter.StrToInt64(SrcUpdateTime)
	err = ScheTaskTime.Updates()
	if err != nil {
