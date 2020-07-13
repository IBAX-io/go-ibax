/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"fmt"
	"net/url"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)

//Install task contract
func VDESrcTaskInstallContractSrc(ctx context.Context, d *daemon) error {
	var (
		blockchain_http      string
		blockchain_ecosystem string
		err                  error
	)

	m := &model.VDESrcTask{}
	SrcTask, err := m.GetAllByContractStateSrc(0) //0 not install，1 installed，2 fail
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
		vde_src_apiAddress := blockchain_http
		vde_src_apiEcosystemID := int64(ecosystemID)

		src := filepath.Join(conf.Config.KeysDir, "PrivateKey")
		// Login
		//err := api.KeyLogin(src, api.ApiEcosystemID)
		gAuth_src, _, gPrivate_src, _, _, err := vde_api.KeyLogin(vde_src_apiAddress, src, vde_src_apiEcosystemID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Login chain failure")
			time.Sleep(time.Millisecond * 2)
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
		_, _, _, err = vde_api.PostTxResult(vde_src_apiAddress, vde_src_apiEcosystemID, gAuth_src, gPrivate_src, ContractName, &form)
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
			time.Sleep(time.Millisecond * 2)
			continue
		}

	} //for

	return nil
}
