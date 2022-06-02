package daemons

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	log "github.com/sirupsen/logrus"
)

type VotingRes struct {
	VoteMsgInfo network.VoteMsg `json:"voteMsgInfo"`
	Err         error           `json:"err"`
}

type VotingTotal struct {
	Data          map[string]VotingRes `json:"data"`
	AgreeQuantity int64                `json:"agreeQuantity"`
	LocalAddress  string               `json:"localAddress"`
}

func ToUpdateMachineStatus(tcpAddress string, ch chan map[string]VotingRes, logger *log.Entry) error {
	localAddress := conf.Config.TCPServer.Str()
	data, err := tcpclient.UpdateMachineStatus(localAddress, tcpAddress, logger)
	voteMsg := &network.VoteMsg{}
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.IOError, "error": err, "tcpAddress": tcpAddress}).Error("sending request error")
		voteMsg.Msg = "tcp connection error"
		voteMsg.LocalAddress = localAddress
		voteMsg.TcpAddress = tcpAddress
		voteMsg.Time = time.Now().UnixMilli()
		ch <- map[string]VotingRes{
			tcpAddress: {*voteMsg, err},
		}
		return err
	}

	err = json.Unmarshal(data, &voteMsg)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.JSONUnmarshallError}).Error("JSONUnmarshallError")
		return err
	}
	ch <- map[string]VotingRes{
		tcpAddress: {*voteMsg, nil},
	}
	return nil
}

func ToBroadcastNodeConnInfo(votingTotal VotingTotal, tcpAddress string, logger *log.Entry) {
	data, err := json.Marshal(votingTotal)
	if err != nil {
		return
	}
	err = tcpclient.BroadcastNodeConnInfo(tcpAddress, data, logger)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.IOError, "error": err, "tcpAddress": tcpAddress}).Error("sending request error")
		return
	}
}

func CandidateNodeVoting(ctx context.Context, d *daemon) error {
	if atomic.CompareAndSwapUint32(&d.atomic, 0, 1) {
		defer atomic.StoreUint32(&d.atomic, 0)
	} else {
		return nil
	}
	var (
		candidateNodes []sqldb.CandidateNode
		err            error
		agreeQuantity  int64
	)
	defer func() {
		d.sleepTime = time.Second * 10
	}()
	candidateNodes, err = sqldb.GetCandidateNode(syspar.SysInt(syspar.NumberNodes))
	if err != nil {
		return err
	}
	if len(candidateNodes) == 0 {
		return nil
	}
	ch := make(chan map[string]VotingRes, len(candidateNodes))
	var wg sync.WaitGroup
	for _, node := range candidateNodes {
		wg.Add(1)
		go func(tcpAddress string) {
			defer wg.Done()
			err = ToUpdateMachineStatus(tcpAddress, ch, d.logger)
		}(node.TcpAddress)
	}
	wg.Wait()

	nodeConnMap := make(map[string]VotingRes, len(ch))
	for i := 0; i < cap(ch); i++ {
		serverVotingInfo, ok := <-ch
		if !ok {
			break
		}
		for tcpAddress, res := range serverVotingInfo {
			if res.Err != nil {
				nodeConnMap[tcpAddress] = res
				continue
			}
			err := checkServerSign(res.VoteMsgInfo)
			if err != nil {
				res.VoteMsgInfo.Msg = "Signature verification failed"
				res.VoteMsgInfo.Agree = false
			}
			if res.VoteMsgInfo.Agree {
				agreeQuantity++
			}
			nodeConnMap[tcpAddress] = res
		}
	}
	close(ch)

	if len(nodeConnMap) > 0 {
		localAddress := conf.Config.TCPServer.Str()
		var wg sync.WaitGroup
		for _, node := range candidateNodes {
			wg.Add(1)
			go func(tcpAddress string) {
				defer wg.Done()
				votingTotal := VotingTotal{Data: nodeConnMap, AgreeQuantity: agreeQuantity, LocalAddress: localAddress}
				ToBroadcastNodeConnInfo(votingTotal, tcpAddress, d.logger)
			}(node.TcpAddress)
		}
		wg.Wait()
	}
	return nil
}

func checkServerSign(serverVoteMsg network.VoteMsg) error {
	candidateNodeSql := &sqldb.CandidateNode{}
	err := candidateNodeSql.GetCandidateNodeByAddress(serverVoteMsg.TcpAddress)
	if err != nil {
		return err
	}
	pk, err := hex.DecodeString(candidateNodeSql.NodePubKey)
	if err != nil {
		return err
	}
	pk = crypto.CutPub(pk)
	_, err = crypto.Verify(pk, []byte(serverVoteMsg.VerifyVoteForSign()), serverVoteMsg.Sign)

	if err != nil {
		return err
	}
	return nil
}
