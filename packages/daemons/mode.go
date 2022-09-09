package daemons

import (
	"encoding/hex"
	"errors"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	log "github.com/sirupsen/logrus"
)

type Model interface {
	GetThisNodePosition() (int64, error)
	GetHostWithMaxID() (hosts []string, err error)
}

type HonorNodeMode struct {
}

func (honorNodeMode *HonorNodeMode) GetThisNodePosition() (int64, error) {
	return syspar.GetNodePositionByPublicKey(syspar.GetNodePubKey())
}
func (honorNodeMode *HonorNodeMode) GetHostWithMaxID() ([]string, error) {
	nbs := node.GetNodesBanService()
	hosts, err := nbs.FilterBannedHosts(syspar.GetRemoteHosts())
	if err != nil {
		log.WithError(err).Error("on filtering banned hosts")
		return nil, err
	}
	return hosts, nil
}

type CandidateNodeMode struct {
}

func (candidateNodeMode *CandidateNodeMode) GetThisNodePosition() (int64, error) {
	return GetCandidateNodePositionByPublicKey()
}
func (candidateNodeMode *CandidateNodeMode) GetHostWithMaxID() ([]string, error) {
	candidateNodes, err := sqldb.GetCandidateNode(syspar.SysInt(syspar.NumberNodes))
	if err != nil {
		log.WithError(err).Error("getting candidate node list")
		return nil, err
	}
	hosts := make([]string, len(candidateNodes))
	for index, node := range candidateNodes {
		hosts[index] = node.TcpAddress
	}

	return hosts, nil
}

type SelectModel struct {
}

func (s *SelectModel) GetThisNodePosition() (int64, error) {
	return s.GetWorkMode().GetThisNodePosition()
}
func (s *SelectModel) GetHostWithMaxID() ([]string, error) {
	return s.GetWorkMode().GetHostWithMaxID()
}

func (s SelectModel) GetWorkMode() Model {
	if syspar.IsHonorNodeMode() {
		return &HonorNodeMode{} //1
	}
	return &CandidateNodeMode{} //2
}

func GetCandidateNodePositionByPublicKey() (int64, error) {
	NodePublicKey := hex.EncodeToString(syspar.GetNodePubKey())
	if len(NodePublicKey) < 1 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node public key is empty")
		return 0, errors.New(`node public key is empty`)
	}
	candidateNode := &sqldb.CandidateNode{}

	if candidateNode.GetCandidateNodeByPublicKey(NodePublicKey) != nil {
		log.WithFields(log.Fields{"error": candidateNode.GetCandidateNodeByPublicKey(NodePublicKey)}).Error("getting candidate node error")
		return 0, candidateNode.GetCandidateNodeByPublicKey(NodePublicKey)
	}

	return candidateNode.ID, nil
}
func GetCandidateNodes() (sqldb.CandidateNodes, error) {
	nodePublicKey := hex.EncodeToString(syspar.GetNodePubKey())
	if len(nodePublicKey) < 1 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node public key is empty")
		return nil, errors.New(`node public key is empty`)
	}
	candidateNodes, err := sqldb.GetCandidateNode(syspar.SysInt(syspar.NumberNodes))
	if err != nil {
		log.WithError(err).Error("getting candidate node error")
		return nil, err
	}
	ret := make(sqldb.CandidateNodes, 0)
	for _, node := range candidateNodes {
		if "04"+nodePublicKey != node.NodePubKey {
			ret = append(ret, node)
		}
	}

	return ret, nil
}
