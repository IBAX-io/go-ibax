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
func VDEScheTaskInstallContractSrc(ctx context.Context, d *daemon) error {
	var (
		blockchain_http      string
		blockchain_ecosystem string
		err                  error
	)

	m := &model.VDEScheTask{}
	ScheTask, err := m.GetAllByContractStateSrc(0) //
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data")
		return err
	}
	if len(ScheTask) == 0 {
		//log.Info("Sche task not found")
		time.Sleep(time.Millisecond * 100)
		return nil
	}

	// deal with task data
	for _, item := range ScheTask {
		//fmt.Println("ScheTask:", item.TaskUUID)
		blockchain_http = item.ContractRunHttp
		blockchain_ecosystem = item.ContractRunEcosystem
		//fmt.Println("ContractRunHttp and ContractRunEcosystem:", blockchain_http, blockchain_ecosystem)
		ecosystemID, err := strconv.Atoi(blockchain_ecosystem)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("encode error")
			time.Sleep(2 * time.Second)
			continue
		}
		//api.ApiAddress = blockchain_http
		//api.ApiEcosystemID = int64(ecosystemID)
		vde_sche_apiAddress := blockchain_http
		vde_sche_apiEcosystemID := int64(ecosystemID)

		src := filepath.Join(conf.Config.KeysDir, "PrivateKey")
		// Login
		//err := api.KeyLogin(src, api.ApiEcosystemID)
		gAuth_sche, _, gPrivate_sche, _, _, err := vde_api.KeyLogin(vde_sche_apiAddress, src, vde_sche_apiEcosystemID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Login chain failure")
			time.Sleep(2 * time.Second)
			continue
		}
		//fmt.Println("Login OK!")

		ContractSrc := item.ContractSrcGet

		form := url.Values{
			`Value`:         {ContractSrc},
			"ApplicationId": {"1"},
			`Conditions`:    {`true`}}

		ContractName := `@1NewContract`
		//_, _, _, err = api.PostTxResult(ContractName, &form)
		_, _, _, err = vde_api.PostTxResult(vde_sche_apiAddress, vde_sche_apiEcosystemID, gAuth_sche, gPrivate_sche, ContractName, &form)
		if err != nil {
			item.ContractStateSrc = 2
			item.ContractStateSrcErr = err.Error()
		} else {
			item.ContractStateSrc = 1
			item.ContractStateSrcErr = ""
		}
		//fmt.Println("Call api.PostTxResult Src OK")

		item.UpdateTime = time.Now().Unix()
		err = item.Updates()
		if err != nil {
			fmt.Println("Update VDEScheTask table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDEScheTask table!")
			time.Sleep(time.Millisecond * 100)
			continue
		}
		time.Sleep(time.Second * 2)
	} //for

	return nil
}

//Install task contract
func VDEScheTaskInstallContractDest(ctx context.Context, d *daemon) error {
	var (
		blockchain_http      string
		blockchain_ecosystem string
		err                  error
	)

	m := &model.VDEScheTask{}
	ScheTask, err := m.GetAllByContractStateDest(0) //
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
		blockchain_http = item.ContractRunHttp
		blockchain_ecosystem = item.ContractRunEcosystem
		//fmt.Println("ContractRunHttp and ContractRunEcosystem:", blockchain_http, blockchain_ecosystem)
		ecosystemID, err := strconv.Atoi(blockchain_ecosystem)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("encode error")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		//api.ApiAddress = blockchain_http
		//api.ApiEcosystemID = int64(ecosystemID)
		vde_sche_apiAddress := blockchain_http
		vde_sche_apiEcosystemID := int64(ecosystemID)

		src := filepath.Join(conf.Config.KeysDir, "PrivateKey")
		// Login
		//err := api.KeyLogin(src, api.ApiEcosystemID)
		gAuth_sche, _, gPrivate_sche, _, _, err := vde_api.KeyLogin(vde_sche_apiAddress, src, vde_sche_apiEcosystemID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Login chain failure")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		//fmt.Println("Login OK!")

		ContractDest := item.ContractDestGet

		form := url.Values{
			`Value`:         {ContractDest},
			"ApplicationId": {"1"},
			`Conditions`:    {`true`}}

		ContractName := `@1NewContract`
		//_, _, _, err = api.PostTxResult(ContractName, &form)
		_, _, _, err = vde_api.PostTxResult(vde_sche_apiAddress, vde_sche_apiEcosystemID, gAuth_sche, gPrivate_sche, ContractName, &form)
		if err != nil {
			item.ContractStateDest = 2
			item.ContractStateDestErr = err.Error()
		} else {
			item.ContractStateDest = 1
			item.ContractStateDestErr = ""
		}
		//fmt.Println("Call api.PostTxResult Dest OK")

		item.UpdateTime = time.Now().Unix()
		err = item.Updates()
		if err != nil {
			fmt.Println("Update VDEScheTask table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update VDEScheTask table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		time.Sleep(time.Second * 2)
	} //for

	return nil
}

//Install task contract
//func VDEScheTaskFromSrcInstallContractSrc(ctx context.Context, d *daemon) error {
//	var (
//		blockchain_http      string
//		blockchain_ecosystem string
//		err                  error
//	)
//
//	m := &model.VDEScheTaskFromSrc{}
//	ScheTask, err := m.GetAllByContractStateSrc(0) //0
//	if err != nil {
//		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data")
//		return err
//	}
//	if len(ScheTask) == 0 {
//		//log.Info("Sche task not found")
//		time.Sleep(time.Millisecond * 100)
//		return nil
//	}
//
//	// deal with task data
//	for _, item := range ScheTask {
//		//fmt.Println("ScheTask:", item.TaskUUID)
//		blockchain_http = item.ContractRunHttp
//		blockchain_ecosystem = item.ContractRunEcosystem
//		//fmt.Println("ContractRunHttp and ContractRunEcosystem:", blockchain_http, blockchain_ecosystem)
//		ecosystemID, err := strconv.Atoi(blockchain_ecosystem)
//		if err != nil {
//			log.WithFields(log.Fields{"error": err}).Error("encode error")
//			time.Sleep(2 * time.Second)
//			continue
//		}
//		//api.ApiAddress = blockchain_http
//		//api.ApiEcosystemID = int64(ecosystemID)
//		vde_sche_apiAddress := blockchain_http
//		vde_sche_apiEcosystemID := int64(ecosystemID)
//
//		src := filepath.Join(conf.Config.KeysDir, "PrivateKey")
//		// Login
//		//err := api.KeyLogin(src, api.ApiEcosystemID)
//		gAuth_sche, _, gPrivate_sche, _, _, err := vde_api.KeyLogin(vde_sche_apiAddress, src, vde_sche_apiEcosystemID)
//		if err != nil {
//			log.WithFields(log.Fields{"error": err}).Error("Login chain failure")
//			time.Sleep(2 * time.Second)
//			continue
//		}
//		//fmt.Println("Login OK!")
//
//		ContractSrc := item.ContractSrcGet
//
//		form := url.Values{
//			`Value`:         {ContractSrc},
//			"ApplicationId": {"1"},
//			`Conditions`:    {`true`}}
//
//		ContractName := `@1NewContract`
//		//_, _, _, err = api.PostTxResult(ContractName, &form)
//		_, _, _, err = vde_api.PostTxResult(vde_sche_apiAddress, vde_sche_apiEcosystemID, gAuth_sche, gPrivate_sche, ContractName, &form)
//		if err != nil {
//			item.ContractStateSrc = 2
//			item.ContractStateSrcErr = err.Error()
//		} else {
//			item.ContractStateSrc = 1
//			item.ContractStateSrcErr = ""
//		}
//		//fmt.Println("Call api.PostTxResult Src OK")
//
//		item.UpdateTime = time.Now().Unix()
//		err = item.Updates()
//		if err != nil {
//			fmt.Println("Update VDEScheTask table err: ", err)
//			log.WithFields(log.Fields{"error": err}).Error("Update VDEScheTask table!")
//			time.Sleep(time.Millisecond * 100)
//			continue
//		}
//		time.Sleep(time.Second * 2)
//	} //for
//	return nil
//}
//
////Install task contract
//func VDEScheTaskFromSrcInstallContractDest(ctx context.Context, d *daemon) error {
//	var (
//		blockchain_http      string
//		blockchain_ecosystem string
//		err                  error
//	)
//
//	m := &model.VDEScheTaskFromSrc{}
//	ScheTask, err := m.GetAllByContractStateDest(0) //0
//	if err != nil {
//		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data")
//		return err
//	}
//	if len(ScheTask) == 0 {
//		//log.Info("Sche task not found")
//		time.Sleep(time.Millisecond * 2)
//		return nil
//	}
//
//	// deal with task data
//	for _, item := range ScheTask {
//		//fmt.Println("ScheTask:", item.TaskUUID)
//		blockchain_http = item.ContractRunHttp
//		blockchain_ecosystem = item.ContractRunEcosystem
//		//fmt.Println("ContractRunHttp and ContractRunEcosystem:", blockchain_http, blockchain_ecosystem)
//		ecosystemID, err := strconv.Atoi(blockchain_ecosystem)
//		if err != nil {
//			log.WithFields(log.Fields{"error": err}).Error("encode error")
//			time.Sleep(time.Millisecond * 2)
//			continue
//		}
//		//api.ApiAddress = blockchain_http
//		//api.ApiEcosystemID = int64(ecosystemID)
//		vde_sche_apiAddress := blockchain_http
//		vde_sche_apiEcosystemID := int64(ecosystemID)
//
//		src := filepath.Join(conf.Config.KeysDir, "PrivateKey")
//		// Login
//		//err := api.KeyLogin(src, api.ApiEcosystemID)
//		gAuth_sche, _, gPrivate_sche, _, _, err := vde_api.KeyLogin(vde_sche_apiAddress, src, vde_sche_apiEcosystemID)
//		if err != nil {
//			log.WithFields(log.Fields{"error": err}).Error("Login chain failure")
//			time.Sleep(time.Millisecond * 2)
//			continue
//		}
//		//fmt.Println("Login OK!")
//
//		ContractDest := item.ContractDestGet
//
//		form := url.Values{
//			`Value`:         {ContractDest},
//			"ApplicationId": {"1"},
//			`Conditions`:    {`true`}}
//
//		ContractName := `@1NewContract`
//		//_, _, _, err = api.PostTxResult(ContractName, &form)
//		_, _, _, err = vde_api.PostTxResult(vde_sche_apiAddress, vde_sche_apiEcosystemID, gAuth_sche, gPrivate_sche, ContractName, &form)
//		if err != nil {
//			item.ContractStateDest = 2
//			item.ContractStateDestErr = err.Error()
//		} else {
//			item.ContractStateDest = 1
//			item.ContractStateDestErr = ""
//		}
//		//fmt.Println("Call api.PostTxResult Dest OK")
//
//		item.UpdateTime = time.Now().Unix()
//		err = item.Updates()
//		if err != nil {
//			fmt.Println("Update VDEScheTask table err: ", err)
//			log.WithFields(log.Fields{"error": err}).Error("Update VDEScheTask table!")
//			time.Sleep(time.Millisecond * 2)
//			continue
//		}
//		time.Sleep(time.Second * 2)
//	} //for
//
//	return nil
//}
