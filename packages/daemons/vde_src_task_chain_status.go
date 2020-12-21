/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/crypto"
	"github.com/IBAX-io/go-ibax/packages/crypto/ecies"

	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)

//Generate a chain request
func VDESrcTaskChainStatus(ctx context.Context, d *daemon) error {
	var (
		err             error
		TaskParms       map[string]interface{}
		vde_src_pubkey  string
		vde_dest_pubkey string
		ok              bool

		myContractSrcGet      string
		myContractSrcGetHash  string
		myContractDestGet     string
		myContractDestGetHash string
	)

	m := &model.VDESrcTask{}
	SrcTask, err := m.GetAllByContractStateAndChainState(1, 0, 0) //0
	if err != nil {
		time.Sleep(time.Millisecond * 2)
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data")
		return err
	}
	if len(SrcTask) == 0 {
		//log.Info("Sche task not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}
	// deal with task data
	for _, item := range SrcTask {
		//fmt.Println("ScheTask:", item.TaskUUID)
		//Generate a chain request
		err = json.Unmarshal([]byte(item.Parms), &TaskParms)
		if err != nil {
			log.Info("Error parsing task parameter")
			log.WithError(err)
			continue
		}

		if vde_src_pubkey, ok = TaskParms["vde_src_pubkey"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("src_vde_pubkey parse error")
			continue
		}
		if vde_dest_pubkey, ok = TaskParms["vde_dest_pubkey"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("vde_dest_pubkey parse error")
			continue
		}

		src_chainstatus_flag := 1
		dest_chainstatus_flag := 1
		//vde_src_pubkey_slice := strings.Split(vde_src_pubkey, ";")

		ContractSrcGetPlusHash := item.ContractSrcGetHash + item.ContractSrcGet
		ContractDestGetPlusHash := item.ContractDestGetHash + item.ContractDestGet

		//fmt.Println("--ContractSrcGetPlusHash ", ContractSrcGetPlusHash)
		//fmt.Println("--ContractDestGetPlusHash ", ContractDestGetPlusHash)

		//
		//fmt.Println("--ContractMode ", item.ContractMode)
		//if item.ContractMode == 2 || item.ContractMode == 3 {
		if item.ContractMode == 3 || item.ContractMode == 4 {

			contractData, err := ecies.EccCryptoKey([]byte(ContractSrcGetPlusHash), vde_src_pubkey)
			if err != nil {
				fmt.Println("error", err)
				log.WithFields(log.Fields{"error": err}).Error("EccCryptoKey error")
				continue
			}
			//fmt.Println("--SRC :", ContractSrcGetPlusHash)
			//fmt.Println("--SRC :", contractData)
			contractDataBase64 := base64.StdEncoding.EncodeToString(contractData)
			myContractSrcGet = contractDataBase64
			//fmt.Println("--SRC Base64:", myContractSrcGet)
			if myContractSrcGetHash, err = crypto.HashHex([]byte(myContractSrcGet)); err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Raw data hash failed")
				fmt.Println("HashHex Raw data hash failed ")
				continue
			}

		} else {
			myContractSrcGet = item.ContractSrcGet
			myContractSrcGetHash = item.ContractSrcGetHash
		}
		//if item.ContractMode == 2 || item.ContractMode == 3 {
		if item.ContractMode == 3 || item.ContractMode == 4 {
			contractData, err := ecies.EccCryptoKey([]byte(ContractDestGetPlusHash), vde_src_pubkey)
			if err != nil {
				fmt.Println("error", err)
				log.WithFields(log.Fields{"error": err}).Error("EccCryptoKey error")
				continue
			}
			//fmt.Println("--DEST :", ContractDestGetPlusHash)
			//fmt.Println("--DEST :", contractData)
			contractDataBase64 := base64.StdEncoding.EncodeToString(contractData)
			myContractDestGet = contractDataBase64
			//fmt.Println("--DEST Base64:", myContractDestGet)
			if myContractDestGetHash, err = crypto.HashHex([]byte(myContractDestGet)); err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Raw data hash failed")
				fmt.Println("HashHex Raw data hash failed ")
				continue
			}

		} else {
			myContractDestGet = item.ContractDestGet
			myContractDestGetHash = item.ContractDestGetHash
		}

		ScheTaskChainStatusSrc := model.VDESrcTaskChainStatus{
			TaskUUID:        item.TaskUUID,
			TaskName:        item.TaskName,
			TaskSender:      item.TaskSender,
			TaskReceiver:    vde_src_pubkey,
			Comment:         item.Comment,
			Parms:           item.Parms,
			TaskType:        item.TaskType,
			TaskState:       item.TaskState,
			ContractSrcName: item.ContractSrcName,
			//ContractSrcGet:       item.ContractSrcGet,
			//ContractSrcGetHash:   item.ContractSrcGetHash,
			ContractSrcGet:     myContractSrcGet,
			ContractSrcGetHash: myContractSrcGetHash,
			ContractDestName:   item.ContractDestName,
			//ContractDestGet:      item.ContractDestGet,
			//ContractDestGetHash:  item.ContractDestGetHash,
			ContractDestGet:      myContractDestGet,
			ContractDestGetHash:  myContractDestGetHash,
			ContractRunHttp:      item.ContractRunHttp,
			ContractRunEcosystem: item.ContractRunEcosystem,
			ContractRunParms:     item.ContractRunParms,
			ContractMode:         item.ContractMode,
			ContractStateSrc:     item.ContractStateSrc,
			ContractStateDest:    item.ContractStateDest,
			CreateTime:           time.Now().Unix()}

		if err = ScheTaskChainStatusSrc.Create(); err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Insert vde_src_task_chain_status table failed")
			src_chainstatus_flag = 0
			time.Sleep(time.Millisecond * 2)
			continue
		}
		fmt.Println("Insert vde_src_task_chain_status table ok")
		vde_dest_pubkey_slice := strings.Split(vde_dest_pubkey, ";")
		for index, vde_dest_pubkey_item := range vde_dest_pubkey_slice {
			//
			//if item.ContractMode == 2 || item.ContractMode == 3 {
			if item.ContractMode == 3 || item.ContractMode == 4 {
				contractData, err := ecies.EccCryptoKey([]byte(ContractSrcGetPlusHash), vde_dest_pubkey_item)
				if err != nil {
					fmt.Println("error", err)
					log.WithFields(log.Fields{"error": err}).Error("EccCryptoKey error")
					continue
				}
				contractDataBase64 := base64.StdEncoding.EncodeToString(contractData)
				myContractSrcGet = contractDataBase64

				if myContractSrcGetHash, err = crypto.HashHex([]byte(myContractSrcGet)); err != nil {
					log.WithFields(log.Fields{"error": err}).Error("Raw data hash failed")
					fmt.Println("HashHex Raw data hash failed ")
					continue
				}

			} else {
				myContractSrcGet = item.ContractSrcGet
				myContractSrcGetHash = item.ContractSrcGetHash
			}
			//if item.ContractMode == 2 || item.ContractMode == 3 {
			if item.ContractMode == 3 || item.ContractMode == 4 {
				contractData, err := ecies.EccCryptoKey([]byte(ContractDestGetPlusHash), vde_dest_pubkey_item)
				if err != nil {
					fmt.Println("error", err)
					log.WithFields(log.Fields{"error": err}).Error("EccCryptoKey error")
					continue
				}
				contractDataBase64 := base64.StdEncoding.EncodeToString(contractData)
				myContractDestGet = contractDataBase64

				if myContractDestGetHash, err = crypto.HashHex([]byte(myContractDestGet)); err != nil {
					log.WithFields(log.Fields{"error": err}).Error("Raw data hash failed")
					fmt.Println("HashHex Raw data hash failed ")
					continue
				}

			} else {
				myContractDestGet = item.ContractDestGet
				myContractDestGetHash = item.ContractDestGetHash
			}

			//Generate data send request
			SrcTaskChainStatusDest := model.VDESrcTaskChainStatus{
				TaskUUID:        item.TaskUUID,
				TaskName:        item.TaskName,
				TaskSender:      item.TaskSender,
				TaskReceiver:    vde_dest_pubkey_item,
				Comment:         item.Comment,
				Parms:           item.Parms,
				TaskType:        item.TaskType,
				TaskState:       item.TaskState,
				ContractSrcName: item.ContractSrcName,
				//ContractSrcGet:       item.ContractSrcGet,
				//ContractSrcGetHash:   item.ContractSrcGetHash,
				ContractSrcGet:     myContractSrcGet,
				ContractSrcGetHash: myContractSrcGetHash,
				ContractDestName:   item.ContractDestName,
				//ContractDestGet:      item.ContractDestGet,
				//ContractDestGetHash:  item.ContractDestGetHash,
				ContractDestGet:      myContractDestGet,
				ContractDestGetHash:  myContractDestGetHash,
				ContractRunHttp:      item.ContractRunHttp,
				ContractRunEcosystem: item.ContractRunEcosystem,
				ContractRunParms:     item.ContractRunParms,
				ContractMode:         item.ContractMode,
				ContractStateSrc:     item.ContractStateSrc,
				ContractStateDest:    item.ContractStateDest,
				CreateTime:           time.Now().Unix()}

			if err = SrcTaskChainStatusDest.Create(); err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Insert vde_src_task_chain_status table failed")
				dest_chainstatus_flag = 0
				time.Sleep(time.Millisecond * 2)
				continue
			}
			fmt.Println("Insert vde_src_task_chain_status table ok:", index)
		} //for
		if src_chainstatus_flag == 1 && dest_chainstatus_flag == 1 {
			item.ChainState = 1
			item.UpdateTime = time.Now().Unix()
			err = item.Updates()
			if err != nil {
				fmt.Println("Update VDESrcTask table err: ", err)
				log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTask table!")
				time.Sleep(time.Millisecond * 2)
				continue
			}
		}

	} //for
	return nil
		vde_src_pubkey  string
		vde_dest_pubkey string
		ok              bool
	)

	m := &model.VDESrcTask{}
	ScheTask, err := m.GetAllByContractStateAndChainState(1, 0, 1) //02
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data")
		return err
	}
	if len(ScheTask) == 0 {
		//log.Info("Sche task not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}
	// deal with task data
	for _, item := range ScheTask {
		//fmt.Println("ScheTask:", item.TaskUUID)
		err = json.Unmarshal([]byte(item.Parms), &TaskParms)
		if err != nil {
			log.Info("Error parsing task parameter")
			log.WithError(err)
			continue
		}

		if vde_src_pubkey, ok = TaskParms["vde_src_pubkey"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("src_vde_pubkey parse error")
			continue
		}
		if vde_dest_pubkey, ok = TaskParms["vde_dest_pubkey"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("vde_dest_pubkey parse error")
			continue
		}

		src_uptochain_flag := 1
		dest_uptochain_flag := 1
		m := &model.VDESrcTaskChainStatus{}
		_, err := m.GetOneByTaskUUIDAndReceiverAndChainState(item.TaskUUID, vde_src_pubkey, 2) // 2
		if err != nil {
			//log.WithFields(log.Fields{"error": err}).Error("getting sche task chain status")
			time.Sleep(time.Millisecond * 2)
			src_uptochain_flag = 0
			continue
		}
		vde_dest_pubkey_slice := strings.Split(vde_dest_pubkey, ";")
		for _, vde_dest_pubkey_item := range vde_dest_pubkey_slice {
			m := &model.VDESrcTaskChainStatus{}
			_, err := m.GetOneByTaskUUIDAndReceiverAndChainState(item.TaskUUID, vde_dest_pubkey_item, 2) // 2
			if err != nil {
				//log.WithFields(log.Fields{"error": err}).Error("getting sche task chain status")
				time.Sleep(time.Millisecond * 2)
				dest_uptochain_flag = 0
				break
			}
		} //for
		if src_uptochain_flag == 1 && dest_uptochain_flag == 1 {
			item.ChainState = 2
			item.UpdateTime = time.Now().Unix()
			err = item.Updates()
			if err != nil {
				fmt.Println("Update VDEScheTask table err: ", err)
				log.WithFields(log.Fields{"error": err}).Error("Update VDEScheTask table!")
				time.Sleep(time.Millisecond * 2)
				continue
			}
		}
	} //for
	return nil
}
