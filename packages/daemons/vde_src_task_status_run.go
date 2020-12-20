/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"path/filepath"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/model"
	vde_api "github.com/IBAX-io/go-ibax/packages/vde_sdk"

	log "github.com/sirupsen/logrus"
)

//Scheduling task information up the chain
func VDESrcTaskStatusRun(ctx context.Context, d *daemon) error {
	var (
		blockchain_http      string
		blockchain_ecosystem string
		Auth                 string
		TaskUUID             string
		Parms                string

		err error

		ContractRunParms  map[string]interface{}
		vde_src_http      string
		vde_src_ecosystem string
		ok                bool
	)

	m := &model.VDESrcTaskStatus{}
	SrcTask, err := m.GetAllByChainState(0) //0 not run
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	if len(SrcTask) == 0 {
		log.Info("Src task to run not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}
	// deal with task data
	for _, item := range SrcTask {
		//fmt.Println("SrcTask:", item.TaskUUID)
		blockchain_http = item.ContractRunHttp
		blockchain_ecosystem = item.ContractRunEcosystem
		//fmt.Println("ContractRun:", blockchain_http, blockchain_ecosystem)

		ecosystemID, err := strconv.Atoi(blockchain_ecosystem)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("blockchain_ecosystem encode error")
			time.Sleep(time.Millisecond * 2)
			continue
		}

		vde_src_apiAddress := blockchain_http
		vde_src_apiEcosystemID := int64(ecosystemID)

		src := filepath.Join(conf.Config.KeysDir, "PrivateKey")
		// Login
		gAuth_src, _, gPrivate_src, _, _, err := vde_api.KeyLogin(vde_src_apiAddress, src, vde_src_apiEcosystemID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Login VDE src chain failure")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		//fmt.Println("Login VDE src OK!")

		err = json.Unmarshal([]byte(item.ContractRunParms), &ContractRunParms)
		if err != nil {
			fmt.Println("Error parsing ContractRunParms parameter!")
			log.Info("Error parsing ContractRunParms parameter")
			log.WithError(err)
			time.Sleep(time.Millisecond * 2)
			continue
		}

		if vde_src_http, ok = ContractRunParms["vde_src_http"].(string); !ok {
			fmt.Println("Error parsing ContractRunParms vde_src_http parameter!")
			log.WithFields(log.Fields{"error": err}).Error("vde_src_http parse error")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		if vde_src_ecosystem, ok = ContractRunParms["vde_src_ecosystem"].(string); !ok {
			fmt.Println("Error parsing ContractRunParms vde_src_ecosystem parameter!")
			log.WithFields(log.Fields{"error": err}).Error("vde_src_ecosystem parse error")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		vde_src_ecosystemID, err := strconv.Atoi(vde_src_ecosystem)
		if err != nil {
			fmt.Println("Error Atoi ContractRunParms vde_src_ecosystem parameter!")
			log.WithFields(log.Fields{"error": err}).Error("vde_src_ecosystem Atoi error")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		vde_src_apiAddress_2 := vde_src_http
		vde_src_apiEcosystemID_2 := int64(vde_src_ecosystemID)
		gAuth_src_2, _, _, _, _, err := vde_api.KeyLogin(vde_src_apiAddress_2, src, vde_src_apiEcosystemID_2)
		if err != nil {
			fmt.Println("error", err)
			time.Sleep(time.Millisecond * 2)
			continue
		}
		//fmt.Println("VDE src 2 Login OK!")

		Auth = gAuth_src_2
		TaskUUID = item.TaskUUID
		Parms = item.ContractRunParms

		form := url.Values{
			"Auth":     {Auth},
			"TaskUUID": {TaskUUID},
			"Parms":    {Parms},
		}

		ContractName := `@1` + item.ContractSrcName
		_, txHash, _, err := vde_api.VDEPostTxResult(vde_src_apiAddress, vde_src_apiEcosystemID, gAuth_src, gPrivate_src, ContractName, &form)
		if err != nil {
			fmt.Println("Run VDESrcTaskContract err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Run VDESrcTaskContract!")

			//
			//item.ChainState = 4
			item.ChainState = 3
			item.ChainErr = err.Error()
			item.UpdateTime = time.Now().Unix()
			err = item.Updates()
			if err != nil {
				fmt.Println("Update VDESrcTaskStatus table err: ", err)
				log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTaskStatus table!")
			}
			time.Sleep(time.Second * 5)
			continue
		}
		fmt.Println("Send VDE src Contract to run, ContractName:", ContractName)

		item.ChainState = 1
		item.TxHash = txHash
		item.BlockId = 0
		item.ChainErr = ""
		item.UpdateTime = time.Now().Unix()
		err = item.Updates()
		if err != nil {
			fmt.Println("Update VDESrcTaskStatus table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTaskStatus table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}
	}
	return nil
}

//Query the status of the chain on the scheduling task information
func VDESrcTaskStatusRunState(ctx context.Context, d *daemon) error {
	var (
		blockchain_http      string
		blockchain_ecosystem string
		err                  error
	)

	m := &model.VDESrcTaskStatus{}
	SrcTask, err := m.GetAllByChainState(1) //1 up to chain
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

	// deal with task data
	for _, item := range SrcTask {
		//fmt.Println("SrcTask:", item.TaskUUID)
		m2 := &model.VDESrcTask{}
		ThisScrTask, err := m2.GetOneByTaskUUID(item.TaskUUID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("VDESrcTask getting one task by TaskUUID")
			time.Sleep(time.Millisecond * 2)
			continue
		//fmt.Println("SrcChainInfo:", blockchain_http, blockchain_ecosystem)

		ecosystemID, err := strconv.Atoi(blockchain_ecosystem)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("blockchain_ecosystem encode error")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		vde_src_apiAddress := blockchain_http
		vde_src_apiEcosystemID := int64(ecosystemID)

		src := filepath.Join(conf.Config.KeysDir, "PrivateKey")
		// Login
		gAuth_src, _, _, _, _, err := vde_api.KeyLogin(vde_src_apiAddress, src, vde_src_apiEcosystemID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Login VDE src chain failure")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		//fmt.Println("Login VDE src OK!")

		blockId, err := vde_api.VDEWaitTx(vde_src_apiAddress, gAuth_src, string(item.TxHash))
		if blockId > 0 {
			//fmt.Println("call src task VDEWaitTx! OK!")
			item.BlockId = blockId
			item.ChainId = converter.StrToInt64(err.Error())
			item.ChainState = 2
			item.ChainErr = ""
			ThisScrTask.TaskRunState = 1
		} else if blockId == 0 {
			//fmt.Println("call src task VDEWaitTx! err: ", item.ChainErr)
			item.ChainState = 3
			item.ChainErr = err.Error()
			ThisScrTask.TaskRunState = 2
			ThisScrTask.TaskRunStateErr = err.Error()
		} else {
			//fmt.Println("call src task VDEWaitTx! err: ", err)
			time.Sleep(time.Millisecond * 2)
			continue
		}

		item.UpdateTime = time.Now().Unix()
		err = item.Updates()
		if err != nil {
			fmt.Println("Update VDESrcTask table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTask table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}

		ThisScrTask.UpdateTime = time.Now().Unix()
		err = ThisScrTask.Updates()
		if err != nil {
			fmt.Println("Update VDESrcTask table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTask table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		fmt.Println("Run VDE src Contract ok, TxHash:", string(item.TxHash))
	} //for
	return nil
}
