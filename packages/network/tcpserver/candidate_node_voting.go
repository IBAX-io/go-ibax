package tcpserver

import (
	"encoding/hex"
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

func CandidateNodeVoting(r *network.CandidateNodeVotingRequest) (*network.CandidateNodeVotingResponse, error) {
	resp := &network.CandidateNodeVotingResponse{}
	voteMsg := &network.VoteMsg{}
	err := json.Unmarshal(r.Data, voteMsg)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("JSONUnmarshallError")
		return nil, err
	}
	voteMsg, err = checkClientVote(r, voteMsg)
	if err != nil {
		log.WithFields(log.Fields{"type": "CheckVote", "error": err}).Error("check vote error")
		return nil, err
	}
	data, err := json.Marshal(voteMsg)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.JSONMarshallError, "error": err}).Error("JSONMarshallError")
		return nil, err
	}
	resp.Data = data
	return resp, nil
}

type VotingRes struct {
	VoteMsgInfo network.VoteMsg `json:"voteMsgInfo"`
	Err         string          `json:"err"`
}
type VotingTotal struct {
	Data          map[string]VotingRes `json:"data"`
	AgreeQuantity int64                `json:"agreeQuantity"`
	LocalAddress  string               `json:"localAddress"`
	St            int64                `json:"st"`
}

func SyncMatchineStateRes(request *network.BroadcastNodeConnInfoRequest) (*network.BroadcastNodeConnInfoResponse, error) {
	resp := &network.BroadcastNodeConnInfoResponse{}
	var votingTotal VotingTotal

	err := json.Unmarshal(request.Data, &votingTotal)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.UnmarshallingError, "error": err}).Error("unmarshal voting total")
		return nil, err
	}
	if votingTotal.AgreeQuantity > 0 {
		candidateNode := &sqldb.CandidateNode{
			TcpAddress:     votingTotal.LocalAddress,
			ReplyCount:     votingTotal.AgreeQuantity,
			DateReply:      votingTotal.St,
			CandidateNodes: request.Data,
		}
		err = candidateNode.UpdateCandidateNodeInfo()
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("update candidate honor voting")
			return nil, err
		}
	}

	return resp, nil
}

func checkClientVote(r *network.CandidateNodeVotingRequest, voteMsgParam *network.VoteMsg) (*network.VoteMsg, error) {
	var (
		prevBlock        = &sqldb.InfoBlock{}
		st               = time.Now()
		candidateNodeSql = &sqldb.CandidateNode{}
	)
	_, err := prevBlock.Get()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting previous block")
		return nil, err
	}
	voteMsg := &network.VoteMsg{
		CurrentBlockHeight: prevBlock.BlockID,
		LocalAddress:       voteMsgParam.LocalAddress,
		TcpAddress:         voteMsgParam.TcpAddress,
		EcosystemID:        0,
		Hash:               prevBlock.Hash,
		Time:               time.Now().UnixMilli(),
	}
	if voteMsgParam.CurrentBlockHeight < prevBlock.BlockID {
		voteMsg.Msg = "Not synced to latest block"
		voteMsg.Agree = false
		signed, err := sign(voteMsg)
		if err != nil {
			return nil, err
		}
		voteMsg.Sign = signed
		return voteMsg, nil
	}
	timeVerification := st.After(time.Unix(voteMsgParam.Time, 0))
	if timeVerification {
		voteMsg.Msg = "Time verification failed"
		voteMsg.Agree = false
		signed, err := sign(voteMsg)
		if err != nil {
			return nil, err
		}
		voteMsg.Sign = signed
		return voteMsg, nil
	}
	err = candidateNodeSql.GetCandidateNodeByAddress(voteMsgParam.LocalAddress)
	if err != nil {
		return nil, err
	}
	pk, err := hex.DecodeString(candidateNodeSql.NodePubKey)
	pk = crypto.CutPub(pk)
	_, err = crypto.Verify(pk, []byte(voteMsgParam.VoteForSign()), voteMsgParam.Sign)
	if err != nil {
		voteMsg.Msg = "Signature verification failed"
		voteMsg.Agree = false
		return voteMsg, nil
	}
	voteMsg.Msg = "Passed the verification"
	voteMsg.Agree = true
	signed, err := sign(voteMsg)
	if err != nil {
		return nil, err
	}
	voteMsg.Sign = signed

	return voteMsg, nil
}

func sign(voteMsg *network.VoteMsg) ([]byte, error) {
	NodePrivateKey, _ := utils.GetNodeKeys()
	if len(NodePrivateKey) < 1 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node private key is empty")
		return nil, errors.New(`node private key is empty`)
	}
	signStr := voteMsg.VerifyVoteForSign()
	signed, err := crypto.SignString(NodePrivateKey, signStr)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("verify voting signature")
		return nil, err
	}

	return signed, nil
}
