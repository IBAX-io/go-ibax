/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/crypto"
	"github.com/IBAX-io/go-ibax/packages/crypto/ecies"

	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)

//Generate a chain request
func VDESrcTaskAuthChainStatus(ctx context.Context, d *daemon) error {
	var (
		err error

		myContractSrcGet      string
		myContractSrcGetHash  string
		myContractDestGet     string
		myContractDestGetHash string
	)

	m := &model.VDESrcTaskAuth{}
	srcTaskAuth, err := m.GetAllByChainState(0) //0
	if err != nil {
		time.Sleep(time.Millisecond * 100)
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated src task auth data")
		return err
	}
	if len(srcTaskAuth) == 0 {
		//log.Info("Src task auth not found")
		time.Sleep(time.Millisecond * 100)
		return nil
	}
	// deal with task data
	for _, item := range srcTaskAuth {
		fmt.Println("srcTaskAuth:", item.TaskUUID, item.VDEPubKey)

		m2 := &model.VDESrcTask{}
		srcTask, err := m2.GetOneByTaskUUID(item.TaskUUID)
		if err != nil {
			fmt.Println("getting src task data err!")
			log.WithFields(log.Fields{"error": err}).Error("getting src task data")
			item.ChainState = 5
			item.UpdateTime = time.Now().Unix()
			err = item.Updates()
			if err != nil {
				fmt.Println("Update VDESrcTaskAuth table err: ", err)
				log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTaskAuth table!")
				time.Sleep(time.Millisecond * 2)
				continue
			}
			time.Sleep(time.Millisecond * 2)
			continue
		}
		//Generate a chain request
		ContractSrcGetPlusHash := srcTask.ContractSrcGetHash + srcTask.ContractSrcGet
		ContractDestGetPlusHash := srcTask.ContractDestGetHash + srcTask.ContractDestGet

		//fmt.Println("--ContractSrcGetPlusHash ", ContractSrcGetPlusHash)
		//fmt.Println("--ContractDestGetPlusHash ", ContractDestGetPlusHash)

		//
		//fmt.Println("--ContractMode ", item.ContractMode)
		//if srcTask.ContractMode == 2 || srcTask.ContractMode == 3 {
		if srcTask.ContractMode == 3 || srcTask.ContractMode == 4 {

			contractData, err := ecies.EccCryptoKey([]byte(ContractSrcGetPlusHash), item.VDEPubKey)
			if err != nil {
				fmt.Println("error", err)
				log.WithFields(log.Fields{"error": err}).Error("EccCryptoKey error")
				continue
			}
			//fmt.Println("--SRC  :", ContractSrcGetPlusHash)
			//fmt.Println("--SRC  :", contractData)
			contractDataBase64 := base64.StdEncoding.EncodeToString(contractData)
			myContractSrcGet = contractDataBase64
			//fmt.Println("--SRC Base64 :", myContractSrcGet)
			if myContractSrcGetHash, err = crypto.HashHex([]byte(myContractSrcGet)); err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Raw data hash failed")
				fmt.Println("HashHex Raw data hash failed ")
				continue
			}

		} else {
			myContractSrcGet = srcTask.ContractSrcGet
			myContractSrcGetHash = srcTask.ContractSrcGetHash
		}
		//if srcTask.ContractMode == 2 || srcTask.ContractMode == 3 {
		if srcTask.ContractMode == 3 || srcTask.ContractMode == 4 {
			contractData, err := ecies.EccCryptoKey([]byte(ContractDestGetPlusHash), item.VDEPubKey)
			if err != nil {
				fmt.Println("error", err)
				log.WithFields(log.Fields{"error": err}).Error("EccCryptoKey error")
				continue
			}
			//fmt.Println("--DEST  :", ContractDestGetPlusHash)
				continue
			}

		} else {
			myContractDestGet = srcTask.ContractDestGet
			myContractDestGetHash = srcTask.ContractDestGetHash
		}

		SrcTaskChainStatusAuth := model.VDESrcTaskChainStatus{
			TaskUUID:        srcTask.TaskUUID,
			TaskName:        srcTask.TaskName,
			TaskSender:      srcTask.TaskSender,
			TaskReceiver:    item.VDEPubKey,
			Comment:         srcTask.Comment,
			Parms:           srcTask.Parms,
			TaskType:        srcTask.TaskType,
			TaskState:       srcTask.TaskState,
			ContractSrcName: srcTask.ContractSrcName,
			//ContractSrcGet:       item.ContractSrcGet,
			//ContractSrcGetHash:   item.ContractSrcGetHash,
			ContractSrcGet:     myContractSrcGet,
			ContractSrcGetHash: myContractSrcGetHash,
			ContractDestName:   srcTask.ContractDestName,
			//ContractDestGet:      item.ContractDestGet,
			//ContractDestGetHash:  item.ContractDestGetHash,
			ContractDestGet:     myContractDestGet,
			ContractDestGetHash: myContractDestGetHash,
			//for sche VDE
			//ContractRunHttp:      srcTask.ContractRunHttp,
			//ContractRunEcosystem: srcTask.ContractRunEcosystem,
			ContractRunHttp:      item.ContractRunHttp,
			ContractRunEcosystem: item.ContractRunEcosystem,
			ContractRunParms:     srcTask.ContractRunParms,
			ContractMode:         srcTask.ContractMode,
			ContractStateSrc:     srcTask.ContractStateSrc,
			ContractStateDest:    srcTask.ContractStateDest,
			CreateTime:           time.Now().Unix()}

		if err = SrcTaskChainStatusAuth.Create(); err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Insert vde_src_task_chain_status table failed")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		fmt.Println("Insert vde_src_task_chain_status table ok")

		item.ChainState = 1
		item.UpdateTime = time.Now().Unix()
		err = item.Updates()
		if err != nil {
			fmt.Println("Update VDESrcTaskAuth table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTaskAuth table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}
	} //for
	return nil
}

//Search a chain request
func VDESrcTaskAuthChainStatusState(ctx context.Context, d *daemon) error {
	var (
		err error
	)

	m := &model.VDESrcTaskAuth{}
	srcTaskAuth, err := m.GetAllByChainState(1) //0
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting src task auth data")
		time.Sleep(time.Millisecond * 100)
		return err
	}
	if len(srcTaskAuth) == 0 {
		//log.Info("src task auth not found")
		time.Sleep(time.Millisecond * 100)
		return nil
	}
	// deal with task data
	for _, item := range srcTaskAuth {
		//fmt.Println("ScheTask:", item.TaskUUID)

		m2 := &model.VDESrcTaskChainStatus{}
		_, err := m2.GetOneByTaskUUIDAndReceiverAndChainState(item.TaskUUID, item.VDEPubKey, 2) // 2
		if err != nil {
			//log.WithFields(log.Fields{"error": err}).Error("getting src task auth chain status")
			time.Sleep(time.Millisecond * 2)
			continue
		}

		item.ChainState = 2
		item.UpdateTime = time.Now().Unix()
		err = item.Updates()
		if err != nil {
			fmt.Println("Update VDEScheTaskAuth table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDEScheTaskAuth table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}
	} //for
	return nil
}
