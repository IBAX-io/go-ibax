package tcpclient

import (
	"encoding/json"
	"errors"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/utils"
	log "github.com/sirupsen/logrus"
	"time"
)

func UpdateMachineStatus(localAddress, tcpAddress string, logger *log.Entry) ([]byte, error) {
	conn, err := newConnection(tcpAddress)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.ConnectionError, "error": err, "tcpAddress": tcpAddress}).Error("dialing to host")
		return nil, err
	}
	defer conn.Close()

	rt := &network.RequestType{Type: network.RequestTypeVoting}
	if err = rt.Write(conn); err != nil {
		logger.WithFields(log.Fields{"type": consts.IOError, "error": err, "tcpAddress": tcpAddress}).Error("sending request type")
		return nil, err
	}

	prevBlock := &sqldb.InfoBlock{}
	_, err = prevBlock.Get()
	NodePrivateKey, NodePublicKey := utils.GetNodeKeys()
	if len(NodePrivateKey) < 1 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node private key is empty")
		return nil, errors.New(`node private key is empty`)
	}
	if len(NodePublicKey) < 1 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node public key is empty")
		return nil, errors.New(`node public key is empty`)
	}

	voteMsg := &network.VoteMsg{
		CurrentBlockHeight: prevBlock.BlockID,
		LocalAddress:       localAddress,
		TcpAddress:         tcpAddress,
		EcosystemID:        0,
		Hash:               prevBlock.Hash,
		Time:               time.Now().UnixMilli(),
	}

	signStr := voteMsg.VoteForSign()
	signed, err := crypto.SignString(NodePrivateKey, signStr)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("signing voteMsg")
		return nil, err
	}
	voteMsg.Sign = signed
	data, err := json.Marshal(voteMsg)
	if err != nil {
		log.Fatalf("VoteMsg JSON marshaling failed: %s", err)
	}

	req := &network.CandidateNodeVotingRequest{
		Data: data,
	}
	if err = req.Write(conn); err != nil {
		logger.WithFields(log.Fields{"type": consts.IOError, "error": err, "tcpAddress": tcpAddress}).Error("sending voting request")
		return nil, err
	}

	resp := &network.CandidateNodeVotingResponse{}
	if err := resp.Read(conn); err != nil {
		logger.WithFields(log.Fields{"type": consts.IOError, "error": err, "tcpAddress": tcpAddress}).Error("receiving voting response")
		return nil, err
	}

	return resp.Data, nil
}

func BroadcastNodeConnInfo(tcpAddress string, data []byte, logger *log.Entry) error {
	conn, err := newConnection(tcpAddress)

	if err != nil {
		logger.WithFields(log.Fields{"type": consts.ConnectionError, "error": err, "tcpAddress": tcpAddress}).Error("dialing to host")
		return err
	}
	defer conn.Close()
	rt := &network.RequestType{Type: network.RequestSyncMatchineState}
	if err = rt.Write(conn); err != nil {
		logger.WithFields(log.Fields{"type": consts.IOError, "error": err, "tcpAddress": tcpAddress}).Error("sending request type")
		return err
	}
	req := &network.BroadcastNodeConnInfoRequest{
		Data: data,
	}
	if err = req.Write(conn); err != nil {
		logger.WithFields(log.Fields{"type": consts.IOError, "error": err, "tcpAddress": tcpAddress}).Error("sending voting request")
		return err
	}
	return nil
}
