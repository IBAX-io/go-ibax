/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/crypto/ecies"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"

	log "github.com/sirupsen/logrus"
)

func SendPrivateData(ctx context.Context, d *daemon) error {
	if atomic.CompareAndSwapUint32(&d.atomic, 0, 1) {
		defer atomic.StoreUint32(&d.atomic, 0)
	} else {
		return nil
	}
	var (
		tcpstr    string
		dist      map[string]interface{}
		found, ok bool
		err       error

	}

	if m.TaskType == "1" { //1 create table，not need to send
		m.TcpSendState = 1
		err = m.Updates()
		if err != nil {
			log.WithError(err)
		}
		return nil
	}

	err = json.Unmarshal([]byte(m.Dist), &dist)
	if err != nil {
		log.WithError(err)
		return nil
	}

	if tran_mode, ok = dist["tran_mode"].(string); !ok {
		log.WithFields(log.Fields{"error": err}).Error("tran mode parse error")
		return nil
	}

	if tcpstr, ok = dist["tcp"].(string); !ok {
		log.WithFields(log.Fields{"error": err}).Error("tcp address parse error")
		return nil
	}

	if node_filename, ok = dist["node_filename"].(string); !ok {
		log.WithFields(log.Fields{"error": err}).Error("node_filename parse error")
		return nil
	}
	if mimetype, ok = dist["mimetype"].(string); !ok {
		//log.WithFields(log.Fields{"error": err}).Error("mimetype parse error, set mimetype = \"application/octet-stream\"")
		mimetype = "application/octet-stream"
		//return nil
	}

	if node_pubkey, ok = dist["node_pubkey"].(string); !ok {
		log.WithFields(log.Fields{"error": err}).Error("node_pubkey parse error")
		return nil
	}
	node_pubkeyslice := strings.Split(node_pubkey, ";")

	tcpslice := strings.Split(tcpstr, ";")
	for key, tcp := range tcpslice {
		if tran_mode == "0" { //Chain down transport mode
			//FileBytes := m.Data
			FileBytes, err := ecies.EccCryptoKey(m.Data, node_pubkeyslice[key])
			if err != nil {
				log.WithError(err)
				continue
			}

			hash := tcpclient.SentPrivateFile(tcp, m.TaskUUID, m.TaskName, m.TaskSender, m.TaskType, node_filename, mimetype, FileBytes)
			if string(hash) == string(m.Hash) {
				m.TcpSendState = 1
				if key < len(m.TcpSendStateFlag) {
					TcpSendStateFlag := []byte(m.TcpSendStateFlag)
					TcpSendStateFlag[key] = '1'
					m.TcpSendStateFlag = string(TcpSendStateFlag)
				}
			} else {
				if key < len(m.TcpSendStateFlag) {
					TcpSendStateFlag := []byte(m.TcpSendStateFlag)
					TcpSendStateFlag[key] = '2'
					m.TcpSendStateFlag = string(TcpSendStateFlag)
				}
			}
			err = m.Updates()
			if err != nil {
				log.WithError(err)
				continue
			}
		} //0
		if tran_mode == "1" { //HASH up chain transport mode
			//DataBytes := m.Data
			DataBytes, err := ecies.EccCryptoKey(m.Data, node_pubkeyslice[key])
			if err != nil {
				log.WithError(err)
				continue
			}

			hash := tcpclient.SentPrivateData(tcp, DataBytes)
			if string(hash) == string(m.Hash) {
				m.TcpSendState = 1
				if key < len(m.TcpSendStateFlag) {
					TcpSendStateFlag := []byte(m.TcpSendStateFlag)
					TcpSendStateFlag[key] = '1'
					m.TcpSendStateFlag = string(TcpSendStateFlag)
				}
				err = m.Updates()
				if err != nil {
					log.WithError(err)
				}
			}
		} //1
		if tran_mode == "2" { //ALL DATA up chain transport mode, not need to send
			m.TcpSendState = 1
			err = m.Updates()
			if err != nil {
				log.WithError(err)
			}
		} //2

	} //for

	return nil
}
