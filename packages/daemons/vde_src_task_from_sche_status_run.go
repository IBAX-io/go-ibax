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

	vde_api "github.com/IBAX-io/go-ibax/packages/vde_sdk"

	"path/filepath"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)

//Scheduling task information up the chain
func VDESrcTaskFromScheStatusRun(ctx context.Context, d *daemon) error {
	var (
		blockchain_http      string
		blockchain_ecosystem string
		Auth                 string
		TaskUUID             string
		Parms                string
		err                  error

		ContractRunParms  map[string]interface{}
		vde_src_http      string
		vde_src_ecosystem string
		ok                bool
	)

	m := &model.VDESrcTaskFromScheStatus{}
	SrcTask, err := m.GetAllByChainState(0) //0
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
		vde_sche_apiAddress := blockchain_http
		vde_sche_apiEcosystemID := int64(ecosystemID)

		src := filepath.Join(conf.Config.KeysDir, "PrivateKey")
		// Login
		gAuth_sche, _, gPrivate_sche, _, _, err := vde_api.KeyLogin(vde_sche_apiAddress, src, vde_sche_apiEcosystemID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Login VDE sche failure")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		//fmt.Println("Login VDE sche OK!")

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
		vde_src_apiAddress := vde_src_http
		vde_src_apiEcosystemID := int64(vde_src_ecosystemID)
		gAuth_src, _, _, _, _, err := vde_api.KeyLogin(vde_src_apiAddress, src, vde_src_apiEcosystemID)
		if err != nil {
			fmt.Println("error", err)
			time.Sleep(time.Millisecond * 2)
			continue
		}
		//fmt.Println("VDE src Login OK!")

		Auth = gAuth_src
		TaskUUID = item.TaskUUID
		Parms = item.ContractRunParms

		form := url.Values{
			"Auth":     {Auth},
			"TaskUUID": {TaskUUID},
			"Parms":    {Parms},
		}

		ContractName := `@1` + item.ContractSrcName
		_, txHash, _, err := vde_api.VDEPostTxResult(vde_sche_apiAddress, vde_sche_apiEcosystemID, gAuth_sche, gPrivate_sche, ContractName, &form)
		if err != nil {
			fmt.Println("Run VDEScheTaskContract err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Run VDEScheTaskContract!")

			//
			item.ChainState = 3
			item.ChainErr = err.Error()
			//time.Sleep(time.Second * 5)
			//continue
		} else {
			fmt.Println("Send VDE sche Contract to run, ContractName:", ContractName)
			item.ChainState = 1
			item.ChainErr = ""
		}

		item.TxHash = txHash
		item.BlockId = 0
		item.UpdateTime = time.Now().Unix()
		err = item.Updates()
		if err != nil {
			fmt.Println("Update VDESrcTaskFromScheStatus table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTaskFromScheStatus table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}

	} //for
	return nil
}

//Query the status of the chain on the scheduling task information
func VDESrcTaskFromScheStatusRunState(ctx context.Context, d *daemon) error {
	var (
		blockchain_http      string
		blockchain_ecosystem string
		err                  error
	)
	}

	// deal with task data
	for _, item := range SrcTask {
		//fmt.Println("SrcTask:", item.TaskUUID)
		m2 := &model.VDESrcTaskFromSche{}
		ThisScrTask, err := m2.GetOneByTaskUUID(item.TaskUUID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("VDESrcTaskFromSche getting one task by TaskUUID")
			time.Sleep(time.Millisecond * 2)
			continue
		}

		blockchain_http = item.ContractRunHttp
		blockchain_ecosystem = item.ContractRunEcosystem
		//fmt.Println("SrcChainInfo:", blockchain_http, blockchain_ecosystem)

		ecosystemID, err := strconv.Atoi(blockchain_ecosystem)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("blockchain_ecosystem encode error")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		vde_sche_apiAddress := blockchain_http
		vde_sche_apiEcosystemID := int64(ecosystemID)

		src := filepath.Join(conf.Config.KeysDir, "PrivateKey")
		// Login
		gAuth_sche, _, _, _, _, err := vde_api.KeyLogin(vde_sche_apiAddress, src, vde_sche_apiEcosystemID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Login VDE sche chain failure")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		//fmt.Println("Login OK!")

		blockId, err := vde_api.VDEWaitTx(vde_sche_apiAddress, gAuth_sche, string(item.TxHash))
		if blockId > 0 {
			//fmt.Println("call sche task VDEWaitTx! OK!")
			item.BlockId = blockId
			item.ChainId = converter.StrToInt64(err.Error())
			item.ChainState = 2
			item.ChainErr = ""
			ThisScrTask.TaskRunState = 1
		} else if blockId == 0 {
			fmt.Println("call sche task VDEWaitTx! err: ", item.ChainErr)
			item.ChainState = 3
			item.ChainErr = err.Error()
			ThisScrTask.TaskRunState = 2
			ThisScrTask.TaskRunStateErr = err.Error()
		} else {
			fmt.Println("call sche task VDEWaitTx! err: ", err)
			time.Sleep(2 * time.Second)
			continue
		}

		item.UpdateTime = time.Now().Unix()
		err = item.Updates()
		if err != nil {
			fmt.Println("Update VDESrcTaskFromScheStatus table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTaskFromScheStatus table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}

		ThisScrTask.UpdateTime = time.Now().Unix()
		err = ThisScrTask.Updates()
		if err != nil {
			fmt.Println("Update VDESrcTaskFromSche table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTaskFromSche table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		fmt.Println("Run VDE sche Contract ok, TxHash:", string(item.TxHash))
	} //for
	return nil
}
