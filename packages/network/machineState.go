package network

import "fmt"

type VoteMsg struct {
	CurrentBlockHeight int64  `json:"currentBlockHeight"`
	LocalAddress       string `json:"localAddress"`
	TcpAddress         string `json:"tcpAddress"`
	EcosystemID        int64  `json:"ecosystemID"`
	Hash               []byte `json:"hash"`
	Agree              bool   `json:"agree"`
	Msg                string `json:"msg"`
	Time               int64  `json:"time"`
	Sign               []byte `json:"sign"`
}

func (voteMsg *VoteMsg) VoteForSign() string {
	return fmt.Sprintf("%v,%v,%v,%v,%x,%v", voteMsg.LocalAddress, voteMsg.TcpAddress, voteMsg.CurrentBlockHeight, voteMsg.EcosystemID, voteMsg.Hash, voteMsg.Time)
}

func (voteMsg *VoteMsg) VerifyVoteForSign() string {
	return fmt.Sprintf("%v,%v,%v,%v,%v,%v,%x,%v", voteMsg.LocalAddress, voteMsg.TcpAddress, voteMsg.CurrentBlockHeight, voteMsg.EcosystemID, voteMsg.Agree, voteMsg.Msg, voteMsg.Hash, voteMsg.Time)
}
