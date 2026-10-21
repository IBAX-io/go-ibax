/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpserver

import (
	"net"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/faults"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/network/bftnet"
	"github.com/IBAX-io/go-ibax/packages/service/node"

	log "github.com/sirupsen/logrus"
)

// HandleTCPRequest proceed TCP requests
func HandleTCPRequest(rw net.Conn) {
	dType := &network.RequestType{}
	err := dType.Read(rw)
	if err != nil {
		log.Errorf("read request type failed: %s", err)
		return
	}

	log.WithFields(log.Fields{"request_type": dType.Type}).Debug("tcpserver got request type")
	var response network.SelfReaderWriter

	switch dType.Type {
	case network.RequestTypeHonorNode:
		if node.IsNodePaused() {
			return
		}
		err = Disseminator(rw)

	case network.RequestTypeNotHonorNode:
		if node.IsNodePaused() {
			return
		}
		err = DisseminateTxs(rw)

	case network.RequestTypeStopNetwork:
		req := &network.StopNetworkRequest{}
		if err = req.Read(rw); err == nil {
			err = StopNetwork(req, rw)
		}

	case network.RequestTypeConfirmation:
		//if node.IsNodePaused() {
		//	return
		//}

		req := &network.ConfirmRequest{}
		if err = req.Read(rw); err == nil {
			response, err = Confirmation(req)
		}

	case network.RequestTypeBlockCollection:
		req := &network.GetBodiesRequest{}
		if err = req.Read(rw); err == nil {
			err = BlockCollection(req, rw)
		}

	case network.RequestTypeMaxBlock:
		response, err = MaxBlock()

	case network.RequestTypeVoting:
		req := &network.CandidateNodeVotingRequest{}
		if err = req.Read(rw); err == nil {
			response, err = CandidateNodeVoting(req)
		}
	case network.RequestSyncMatchineState:
		req := &network.BroadcastNodeConnInfoRequest{}
		if err = req.Read(rw); err == nil {
			_, err = SyncMatchineStateRes(req)
			if err != nil {
				log.WithFields(log.Fields{"type": "SyncMatchineStateRes", "error": err}).Error("SyncMatchineStateRes hour candidate voting")
			}
			return
		}

	case network.RequestTypeBftMessages:
		if s := bftnet.Current(); s != nil {
			if err = s.Serve(rw); err != nil {
				log.WithFields(log.Fields{"type": consts.NetworkError, "error": err, "host": rw.RemoteAddr()}).Debug("bft stream closed")
			}
		}
		return

	case network.RequestTypeBftCertificates:
		if s := bftnet.Current(); s != nil {
			rw.SetDeadline(time.Now().Add(consts.ReadTimeout * time.Second))
			if err = s.ServeCertificates(rw); err != nil {
				log.WithFields(log.Fields{"type": consts.NetworkError, "error": err, "host": rw.RemoteAddr()}).Debug("bft certificates request")
			}
		}
		return
	}

	if err != nil || response == nil {
		return
	}

	log.WithFields(log.Fields{"response": response, "request_type": dType.Type}).Debug("tcpserver responded")
	if err = response.Write(rw); err != nil {
		// err = SendRequest(response, rw)
		log.Errorf("tcpserver handle error: %s", err)
	}
}

// TcpListener is listening tcp address
func TcpListener(laddr string) error {

	if strings.HasPrefix(laddr, "127.") {
		log.Warn("Listening at local address: ", laddr)
	}

	l, err := net.Listen("tcp", laddr)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ConnectionError, "error": err, "host": laddr}).Error("Error listening")
		return err
	}

	go func() {
		defer l.Close()
		for {
			conn, err := l.Accept()
			if err != nil {
				log.WithFields(log.Fields{"type": consts.ConnectionError, "error": err, "host": laddr}).Error("Error accepting")
				time.Sleep(time.Second)
			} else {
				go func(conn net.Conn) {
					HandleTCPRequest(conn)
					conn.Close()
				}(faults.Accepted(conn))
			}
		}
	}()

	return nil
}
