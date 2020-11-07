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
	"strings"
	"time"

	chain_api "github.com/IBAX-io/go-ibax/packages/chain_sdk"

	"path/filepath"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)

//Install task channel
func SubNodeSrcTaskInstallChannel(ctx context.Context, d *daemon) error {
	var (
		TaskParms map[string]interface{}
		//subnode_src_pubkey       string
		subnode_dest_pubkey string
		//subnode_dest_ip          string
		//subnode_agent_pubkey     string
		//subnode_agent_ip         string
		//agent_mode           string
		tran_mode string
		//log_mode             string

		blockchain_table     string
		blockchain_http      string
		blockchain_ecosystem string
		err                  error
		ok                   bool
	)

	m := &model.SubNodeSrcTask{}
	SrcTask, err := m.GetAllByChannelState(0) //0 not install，1 success，2 fail
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
		fmt.Println("SrcTask:", item.TaskUUID)

		err = json.Unmarshal([]byte(item.Parms), &TaskParms)
		if err != nil {
			log.Info("Error parsing task parameter")
			log.WithError(err)
			item.ChannelState = 3 //fail
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if tran_mode, ok = TaskParms["tran_mode"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("hash_mode parse error")
			item.ChannelState = 3 //fail
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}

		if tran_mode == "3" { //Under the chain tran
			item.ChannelState = 4 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}

		//if subnode_src_pubkey, ok = TaskParms["subnode_src_pubkey"].(string); !ok {
		//	log.WithFields(log.Fields{"error": err}).Error("subnode_src_pubkey parse error")
		//	item.ChannelState = 3 //
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	continue
		//}
		if subnode_dest_pubkey, ok = TaskParms["subnode_dest_pubkey"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("subnode_dest_pubkey parse error")
			item.ChannelState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		node_pubkey_slice := strings.Split(subnode_dest_pubkey, ";")

		//if subnode_dest_ip, ok = TaskParms["subnode_dest_ip"].(string); !ok {
		//	log.WithFields(log.Fields{"error": err}).Error("subnode_dest_ip parse error")
		//	item.ChannelState = 3 //Indicates an error in parsing task parameters
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	continue
		//}
		//if subnode_agent_pubkey, ok = TaskParms["subnode_agent_pubkey"].(string); !ok {
		//	log.WithFields(log.Fields{"error": err}).Error("subnode_agent_pubkey parse error")
		//	item.ChannelState = 3 //
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	continue
		//}
		//if subnode_agent_ip, ok = TaskParms["subnode_agent_ip"].(string); !ok {
		//	log.WithFields(log.Fields{"error": err}).Error("subnode_agent_ip parse error")
		//	item.ChannelState = 3 //Indicates an error in parsing task parameters
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	continue
		//}
		//if agent_mode, ok = TaskParms["agent_mode"].(string); !ok {
		//	log.WithFields(log.Fields{"error": err}).Error("agent_mode parse error")
		//	item.ChannelState = 3 //Indicates an error in parsing task parameters
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	continue
		//}

		//if log_mode, ok = TaskParms["log_mode"].(string); !ok {
		//	log.WithFields(log.Fields{"error": err}).Error("log_mode parse error")
		//	item.ChannelState = 3 //Indicates an error in parsing task parameters
		//	err = item.Updates()
		//	if err != nil {
		//		log.WithError(err)
		//	}
		//	continue
		//}
		if blockchain_table, ok = TaskParms["blockchain_table"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("blockchain_table parse error")
			item.ChannelState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if blockchain_http, ok = TaskParms["blockchain_http"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("blockchain_http parse error")
			item.ChannelState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		if blockchain_ecosystem, ok = TaskParms["blockchain_ecosystem"].(string); !ok {
			log.WithFields(log.Fields{"error": err}).Error("blockchain_ecosystem parse error")
			item.ChannelState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}

		//blockchain_http = item.ContractRunHttp
		//blockchain_ecosystem = item.ContractRunEcosystem
		//fmt.Println("ContractRunHttp and ContractRunEcosystem:", blockchain_http, blockchain_ecosystem)
		ecosystemID, err := strconv.Atoi(blockchain_ecosystem)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("encode error")
			item.ChannelState = 3 //Indicates an error in parsing task parameters
			err = item.Updates()
			if err != nil {
				log.WithError(err)
			}
			continue
		}
		//api.ApiAddress = blockchain_http
		//api.ApiEcosystemID = int64(ecosystemID)
		chain_apiAddress := blockchain_http
		chain_apiEcosystemID := int64(ecosystemID)

		src := filepath.Join(conf.Config.KeysDir, "chain_PrivateKey")
		// Login
		//err := api.KeyLogin(src, api.ApiEcosystemID)
		gAuth_chain, _, gPrivate_chain, _, _, err := chain_api.KeyLogin(chain_apiAddress, src, chain_apiEcosystemID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Login chain failure")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		fmt.Println("Login chain OK!")

		form := url.Values{}
		if tran_mode == "1" { //1 hash up to chain
			form = url.Values{
				"Name":          {blockchain_table},
				"ColumnsArr":    {`["task_uuid","data_uuid","data_info","hash","spphdata","deleted","date_created","date_updated","date_deleted"]`},
				"TypesArr":      {`["text","text","json","text","text","number","number","number","number"]`},
				"InsertPerm":    {`true`},
				"NewColumnPerm": {`true`},
				"ReadPerm":      {`1`},
				"UpdatePerm":    {`true`},
				"ApplicationId": {`1`},
			}
		} else if tran_mode == "2" { //2 all data up to chain
			form = url.Values{
				"Name":          {blockchain_table},
				"ColumnsArr":    {`["task_uuid","data_uuid","data_info","hash","sppadata","deleted","date_created","date_updated","date_deleted"]`},
				"TypesArr":      {`["text","text","json","text","text","number","number","number","number"]`},
				"InsertPerm":    {`true`},
				"NewColumnPerm": {`true`},
				"ReadPerm":      {`1`},
				"UpdatePerm":    {`true`},
				"ApplicationId": {`1`},
			}
		} else {
			log.WithFields(log.Fields{"error": err}).Error("tran_mode error")
			continue
		}

		chain_api.ApiPrivateFor = []string{
			tran_mode,
			//"1",
			//node_pubkey,
		}
		chain_api.ApiPrivateFor = append(chain_api.ApiPrivateFor, node_pubkey_slice...)

		ContractName := `@1NewTableJoint`
		_, _, _, err = chain_api.PostTxResult(chain_apiAddress, chain_apiEcosystemID, gAuth_chain, gPrivate_chain, ContractName, &form)
		if err != nil {
			item.ChannelState = 2
			item.ChannelStateErr = err.Error()
		} else {
			item.ChannelState = 1
			item.ChannelStateErr = ""
		}
		fmt.Println("Call chain api.PostTxResult OK")

		item.UpdateTime = time.Now().Unix()
		err = item.Updates()
		if err != nil {
			fmt.Println("Update SubNodeSrcTask table err: ", err)
			log.WithFields(log.Fields{"error": err}).Error("Update SubNodeSrcTask table!")
			time.Sleep(time.Millisecond * 2)
			continue
		}
		fmt.Println("Update SubNodeSrcTask table OK")
	} //for

	return nil
}
