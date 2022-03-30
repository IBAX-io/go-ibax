/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/api"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"

	log "github.com/sirupsen/logrus"
)

const (
	errExternalNone    = iota // 0 - no error
	errExternalTx             // 1 - tx error
	errExternalAttempt        // 2 - attempt error
	errExternalTimeout        // 3 - timeout of getting txstatus

	maxAttempts           = 10
	statusTimeout         = 60
	externalDeamonTimeout = 2
	apiExt                = `/api/v2/`
)

var (
	nodePrivateKey []byte
	nodeKeyID      int64
	nodePublicKey  string
	authNet        = map[string]string{}
)

func loginNetwork(urlPath string) (connect *api.Connect, err error) {
	if len(nodePrivateKey) == 0 {
		var pubKey []byte
		nodePrivateKey = syspar.GetNodePrivKey()
		if pubKey, err = crypto.PrivateToPublic(nodePrivateKey); err != nil {
			return
		}
		nodeKeyID = crypto.Address(pubKey)
		nodePublicKey = crypto.PubToHex(pubKey)
	}
	connect = &api.Connect{
		Auth:       authNet[urlPath],
		PrivateKey: nodePrivateKey,
		PublicKey:  nodePublicKey,
		Root:       urlPath,
	}
	if err = connect.Login(); err != nil {
		authNet[urlPath] = connect.Auth
	}
	return
}

func SendExternalTransaction() error {
	var (
		err     error
		connect *api.Connect
		delList []int64
		hash    string
	)

	toWait := map[string][]sqldb.ExternalBlockchain{}
	incAttempt := func(id int64) {
		if err = sqldb.IncExternalAttempt(id); err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("IncAttempt")
		}
	}
	sendResult := func(item sqldb.ExternalBlockchain, block, errCode int64, resText string) {
		defer func() {
			delList = append(delList, item.Id)
		}()
		if len(item.ResultContract) == 0 {
			return
		}
		if err := transaction.CreateContract(item.ResultContract, nodeKeyID,
			map[string]any{
				"Status": errCode,
				"Msg":    resText,
				"Block":  block,
				"UID":    item.Uid,
			}, nodePrivateKey); err != nil {
			log.WithFields(log.Fields{"type": consts.ContractError, "err": err}).Error("CreateContract")
		}
	}
	list, err := sqldb.GetExternalList()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("GetExternalList")
		return err
	}
	timeOut := time.Now().Unix() - 10*(syspar.GetGapsBetweenBlocks()+
		syspar.GetMaxBlockGenerationTime()/1000)
	for _, item := range list {
		root := item.Url + apiExt
		if item.Sent == 0 {
			if timeOut > item.TxTime {
				delList = append(delList, item.Id)
				continue
			}
			if connect, err = loginNetwork(root); err != nil {
				log.WithFields(log.Fields{"type": consts.AccessDenied, "error": err}).Error("loginNetwork")
				return err
			}
			values := url.Values{"UID": {item.Uid}}

			var params map[string]any
			if err = json.Unmarshal([]byte(item.Value), &params); err != nil {
				log.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("Unmarshal params")
				delList = append(delList, item.Id)
				continue
			}
			for key, val := range params {
				values[key] = []string{fmt.Sprint(val)}
			}
			values["nowait"] = []string{"1"}
			values["txtime"] = []string{converter.Int64ToStr(item.TxTime)}
			_, hash, err = connect.PostTxResult(item.ExternalContract, &values)
			if err != nil {
				log.WithFields(log.Fields{"type": consts.NetworkError, "error": err}).Error("PostContract")
				if item.Attempts >= maxAttempts-1 {
					sendResult(item, 0, errExternalAttempt, ``)
				} else {
					incAttempt(item.Id)
				}
			} else {
				log.WithFields(log.Fields{"hash": hash, "txtime": values["txtime"][0],
					"nodeKey": converter.Int64ToStr(nodeKeyID)}).Info("SendExternalTransaction")
				bHash, err := hex.DecodeString(hash)
				if err != nil {
					log.WithFields(log.Fields{"type": consts.ParseError, "error": err}).Error("DecodeHex")
					incAttempt(item.Id)
				} else if err = sqldb.HashExternalTx(item.Id, bHash); err != nil {
					log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("HashExternal")
				}
			}
		} else {
			toWait[item.Url] = append(toWait[item.Url], item)
		}
	}
	for _, waitList := range toWait {
		if connect, err = loginNetwork(waitList[0].Url + apiExt); err != nil {
			log.WithFields(log.Fields{"type": consts.AccessDenied, "error": err}).Error("loginNetwork")
			continue
		}
		var hashes []string
		for _, item := range waitList {
			hashes = append(hashes, hex.EncodeToString(item.Hash))
		}
		results, err := connect.WaitTxList(hashes)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.NetworkError, "error": err}).Error("WaitTxList")
			continue
		}
		timeOut = time.Now().Unix() - statusTimeout
		for _, item := range waitList {
			if result, ok := results[hex.EncodeToString(item.Hash)]; ok {
				errCode := int64(errExternalNone)
				if result.BlockID == 0 {
					errCode = errExternalTx
				}
				sendResult(item, result.BlockID, errCode, result.Msg)
			} else if timeOut > item.TxTime {
				sendResult(item, 0, errExternalTimeout, ``)
			}
		}
	}
	if len(delList) > 0 {
		if err = sqldb.DelExternalList(delList); err != nil {
			return err
		}
	}
	return nil
}

// ExternalNetwork sends txinfo to the external network
func ExternalNetwork(ctx context.Context, d *daemon) error {
	if atomic.CompareAndSwapUint32(&d.atomic, 0, 1) {
		defer atomic.StoreUint32(&d.atomic, 0)
	} else {
		return nil
	}
	d.sleepTime = externalDeamonTimeout * time.Second
	return SendExternalTransaction()
}
