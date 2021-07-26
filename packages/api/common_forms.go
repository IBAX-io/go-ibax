/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

const (
	defaultPaginatorLimit = 25
	maxPaginatorLimit     = 1000
)

type paginatorForm struct {
	defaultLimit int

	Limit  int `schema:"limit"`
	Offset int `schema:"offset"`
}

func (f *paginatorForm) Validate(r *http.Request) error {
	if f.Limit <= 0 {
		f.Limit = f.defaultLimit
		if f.Limit == 0 {
			f.Limit = defaultPaginatorLimit
		}
	}

	if f.Limit > maxPaginatorLimit {
		f.Limit = maxPaginatorLimit
	}

	return nil
}

type paramsForm struct {
	nopeValidator
	Names string `schema:"names"`
}

func (f *paramsForm) AcceptNames() map[string]bool {
	names := make(map[string]bool)
	for _, item := range strings.Split(f.Names, ",") {
		if len(item) == 0 {
			continue
		}
		names[item] = true
	}
	return names
}

type ecosystemForm struct {
	EcosystemID     int64  `schema:"ecosystem"`
	EcosystemPrefix string `schema:"-"`
	Validator       types.EcosystemIDValidator
}

func (f *ecosystemForm) Validate(r *http.Request) error {
	if f.Validator == nil {
		panic("ecosystemForm.Validator should not be empty")
	}

	client := getClient(r)
	logger := getLogger(r)

	ecosysID, err := f.Validator.Validate(f.EcosystemID, client.EcosystemID, logger)
	if err != nil {
		if err == ErrEcosystemNotFound {
			err = errEcosystem.Errorf(f.EcosystemID)
		}
		return err
	}

	f.EcosystemID = ecosysID
	f.EcosystemPrefix = converter.Int64ToStr(f.EcosystemID)

	return nil
}

//type shareTaskForm struct {
//	Name       string `schema:"name"`
//	ShareType  string `schema:"share_type"`
//	Source     string `schema:"source"`
//	Dist       string `schema:"dist"`
//	Creator    string `schema:"creator"`
//	CreateTime int64  `schema:"create_time"`
//}
//
//func (f *shareTaskForm) Validate(r *http.Request) error {
//	return nil
//}

type shareDataForm struct {
	TaskUUID         string `schema:"task_uuid"`
	TaskName         string `schema:"task_name"`
	TaskSender       string `schema:"task_sender"`
	TaskType         string `schema:"task_type"`
	Hash             string `schema:"hash"`
	Data             string `schema:"data"`
	Dist             string `schema:"dist"`
	Ecosystem        int64  `schema:"ecosystem"`
	TcpSendState     int64  `schema:"tcp_send_state"`
	TcpSendStateFlag string `schema:"tcp_send_state_flag"`
	ChainState       int64  `schema:"chain_state"`
	BlockID          int64  `schema:"block_id"`
	TxHash           string `schema:"tx_hash"`
}

func (f *shareDataForm) Validate(r *http.Request) error {
	return nil
}

//
type SubNodeSrcTaskForm struct {
	TaskUUID   string `schema:"task_uuid"`
	TaskName   string `schema:"task_name"`
	TaskSender string `schema:"task_sender"`
	Comment    string `schema:"comment"`
	Parms      string `schema:"parms"`
	TaskType   int64  `schema:"task_type"`
	TaskState  int64  `schema:"task_state"`

	TaskRunParms    string `schema:"task_run_parms"`
	TaskRunState    int64  `schema:"task_run_state"`
	TaskRunStateErr string `schema:"task_run_state_err"`

	//TxHash     string `schema:"tx_hash"`
	//ChainState int64  `schema:"chain_state"`
	//BlockId    int64  `schema:"block_id"`
	//ChainId    int64  `schema:"chain_id"`
	//ChainErr   string `schema:"chain_err"`
}

func (f *SubNodeSrcTaskForm) Validate(r *http.Request) error {
	return nil
}

type SubNodeSrcDataForm struct {
	TaskUUID string `schema:"task_uuid"`
	DataUUID string `schema:"data_uuid"`
	Hash     string `schema:"hash"`
	Data     string `schema:"data"`
	DataInfo string `schema:"data_info"`
	//DataState int64  `schema:"data_state"`
	//DataErr   string `schema:"data_err"`
}

func (f *SubNodeSrcDataForm) Validate(r *http.Request) error {
	return nil
}

type VDESrcDataForm struct {
	TaskUUID  string `schema:"task_uuid"`
	DataUUID  string `schema:"data_uuid"`
	Hash      string `schema:"hash"`
	Data      string `schema:"data"`
	DataInfo  string `schema:"data_info"`
	DataState int64  `schema:"data_state"`
	DataErr   string `schema:"data_err"`
}

func (f *VDESrcDataForm) Validate(r *http.Request) error {
	return nil
}

type VDESrcTaskForm struct {
	TaskUUID   string `schema:"task_uuid"`
	TaskName   string `schema:"task_name"`
	TaskSender string `schema:"task_sender"`
	Comment    string `schema:"comment"`
	Parms      string `schema:"parms"`
	TaskType   int64  `schema:"task_type"`
	TaskState  int64  `schema:"task_state"`

	ContractSrcName     string `schema:"contract_src_name"`
	ContractSrcGet      string `schema:"contract_src_get"`
	ContractSrcGetHash  string `schema:"contract_src_get_hash"`
	ContractDestName    string `schema:"contract_dest_name"`
	ContractDestGet     string `schema:"contract_dest_get"`
	ContractDestGetHash string `schema:"contract_dest_get_hash"`
	ContractMode        int64  `schema:"contract_mode"`

	ContractStateSrc     int64  `schema:"contract_state_src"`
	ContractStateDest    int64  `schema:"contract_state_dest"`
	TxHash     string `schema:"tx_hash"`
	ChainState int64  `schema:"chain_state"`
	BlockId    int64  `schema:"block_id"`
	ChainId    int64  `schema:"chain_id"`
	ChainErr   string `schema:"chain_err"`
}

func (f *VDESrcTaskForm) Validate(r *http.Request) error {
	return nil
}

type VDESrcTaskFromScheForm struct {
	TaskUUID   string `schema:"task_uuid"`
	TaskName   string `schema:"task_name"`
	TaskSender string `schema:"task_sender"`
	Comment    string `schema:"comment"`
	Parms      string `schema:"parms"`
	TaskType   int64  `schema:"task_type"`
	TaskState  int64  `schema:"task_state"`

	ContractSrcName     string `schema:"contract_src_name"`
	ContractSrcGet      string `schema:"contract_src_get"`
	ContractSrcGetHash  string `schema:"contract_src_get_hash"`
	ContractDestName    string `schema:"contract_dest_name"`
	ContractDestGet     string `schema:"contract_dest_get"`
	ContractDestGetHash string `schema:"contract_dest_get_hash"`
	ContractMode        int64  `schema:"contract_mode"`

	ContractStateSrc     int64  `schema:"contract_state_src"`
	ContractStateDest    int64  `schema:"contract_state_dest"`
	ContractStateSrcErr  string `schema:"contract_state_src_err"`
	ContractStateDestErr string `schema:"contract_state_dest_err"`

	ContractRunHttp      string `schema:"contract_run_http"`
	ContractRunEcosystem string `schema:"contract_run_ecosystem"`
	ContractRunParms     string `schema:"contract_run_parms"`

	TaskRunState    int64  `schema:"task_run_state"`
	TaskRunStateErr string `schema:"task_run_state_err"`

	//TxHash     string `schema:"tx_hash"`
	//ChainState int64  `schema:"chain_state"`
	//BlockId    int64  `schema:"block_id"`
	//ChainId    int64  `schema:"chain_id"`
	//ChainErr   string `schema:"chain_err"`
}

func (f *VDESrcTaskFromScheForm) Validate(r *http.Request) error {
	return nil
}

type VDEScheTaskForm struct {
	TaskUUID   string `schema:"task_uuid"`
	TaskName   string `schema:"task_name"`
	TaskSender string `schema:"task_sender"`
	Comment    string `schema:"comment"`
	Parms      string `schema:"parms"`
	TaskType   int64  `schema:"task_type"`
	TaskState  int64  `schema:"task_state"`

	ContractSrcName     string `schema:"contract_src_name"`
	ContractSrcGet      string `schema:"contract_src_get"`
	ContractSrcGetHash  string `schema:"contract_src_get_hash"`
	ContractDestName    string `schema:"contract_dest_name"`
	ContractDestGet     string `schema:"contract_dest_get"`
	ContractDestGetHash string `schema:"contract_dest_get_hash"`

	ContractRunHttp      string `schema:"contract_run_http"`
	ContractRunEcosystem string `schema:"contract_run_ecosystem"`
	ContractRunParms     string `schema:"contract_run_parms"`

	ContractMode         int64  `schema:"contract_mode"`
	ContractStateSrc     int64  `schema:"contract_state_src"`
	ContractStateDest    int64  `schema:"contract_state_dest"`
	ContractStateSrcErr  string `schema:"contract_state_src_err"`
	ContractStateDestErr string `schema:"contract_state_dest_err"`

	TaskRunState    int64  `schema:"task_run_state"`
	TaskRunStateErr string `schema:"task_run_state_err"`

	TxHash     string `schema:"tx_hash"`
	ChainState int64  `schema:"chain_state"`
	BlockId    int64  `schema:"block_id"`
	ChainId    int64  `schema:"chain_id"`
	ChainErr   string `schema:"chain_err"`
}

func (f *VDEScheTaskForm) Validate(r *http.Request) error {
	return nil
}

type VDESrcChainInfoForm struct {
	BlockchainHttp      string `schema:"blockchain_http"`
	BlockchainEcosystem string `schema:"blockchain_ecosystem"`
	Comment             string `schema:"comment"`
}

func (f *VDESrcChainInfoForm) Validate(r *http.Request) error {
	return nil
}

type VDEScheChainInfoForm struct {
	BlockchainHttp      string `schema:"blockchain_http"`
	BlockchainEcosystem string `schema:"blockchain_ecosystem"`
	Comment             string `schema:"comment"`
}

func (f *VDEScheChainInfoForm) Validate(r *http.Request) error {
	return nil
}

type VDEDestChainInfoForm struct {
	BlockchainHttp      string `schema:"blockchain_http"`
	BlockchainEcosystem string `schema:"blockchain_ecosystem"`
	Comment             string `schema:"comment"`
}

func (f *VDEDestChainInfoForm) Validate(r *http.Request) error {
	return nil
}

type VDEDestDataStatusForm struct {
	TaskUUID       string `schema:"task_uuid"`
	DataUUID       string `schema:"data_uuid"`
	Hash           string `schema:"hash"`
	Data           string `schema:"data"`
	DataInfo       string `schema:"data_info"`
	VDESrcPubkey   string `schema:"vde_src_pubkey"`
	VDEDestPubkey  string `schema:"vde_dest_pubkey"`
	VDEDestIp      string `schema:"vde_dest_ip"`
	VDEAgentPubkey string `schema:"vde_agent_pubkey"`
	VDEAgentIp     string `schema:"vde_agent_ip"`
	AgentMode      int64  `schema:"agent_mode"`
	AuthState      int64  `schema:"auth_state"`
	SignState      int64  `schema:"sign_state"`
	HashState      int64  `schema:"hash_state"`
}

func (f *VDEDestDataStatusForm) Validate(r *http.Request) error {
	return nil
}

type ListVDEDestDataStatusForm struct {
	TaskUUID  string `schema:"task_uuid"`
	BeginTime int64  `schema:"begin_time"`
	EndTime   int64  `schema:"end_time"`
}

func (f *ListVDEDestDataStatusForm) Validate(r *http.Request) error {
	return nil
}

type VDEAgentChainInfoForm struct {
	BlockchainHttp      string `schema:"blockchain_http"`
	BlockchainEcosystem string `schema:"blockchain_ecosystem"`
	Comment             string `schema:"comment"`
	LogMode             int64  `schema:"log_mode"`
}

func (f *VDEAgentChainInfoForm) Validate(r *http.Request) error {
	return nil
}

type VDESrcMemberForm struct {
	VDEPubKey            string `schema:"vde_pub_key"`
	VDEComment           string `schema:"vde_comment"`
	VDEName              string `schema:"vde_name"`
	VDEIp                string `schema:"vde_ip"`
	VDEType              int64  `schema:"vde_type"`
	ContractRunHttp      string `schema:"contract_run_http"`
	ContractRunEcosystem string `schema:"contract_run_ecosystem"`
}

func (f *VDESrcMemberForm) Validate(r *http.Request) error {
	return nil
}

type VDEScheMemberForm struct {
	VDEPubKey            string `schema:"vde_pub_key"`
	VDEComment           string `schema:"vde_comment"`
	VDEName              string `schema:"vde_name"`
	VDEIp                string `schema:"vde_ip"`
	VDEType              int64  `schema:"vde_type"`
	ContractRunHttp      string `schema:"contract_run_http"`
	ContractRunEcosystem string `schema:"contract_run_ecosystem"`
}

func (f *VDEScheMemberForm) Validate(r *http.Request) error {
	return nil
}

type VDESrcTaskAuthForm struct {
	TaskUUID             string `schema:"task_uuid"`
	Comment              string `schema:"vcomment"`
	VDEPubKey            string `schema:"vde_pub_key"`
	ContractRunHttp      string `schema:"contract_run_http"`
	ContractRunEcosystem string `schema:"contract_run_ecosystem"`
	ChainState           int64  `schema:"chain_state"`
}

func (f *VDESrcTaskAuthForm) Validate(r *http.Request) error {
	return nil
}

type VDEAgentMemberForm struct {
	VDEPubKey            string `schema:"vde_pub_key"`
	VDEComment           string `schema:"vde_comment"`
	VDEName              string `schema:"vde_name"`
	VDEIp                string `schema:"vde_ip"`
	VDEType              int64  `schema:"vde_type"`
	ContractRunHttp      string `schema:"contract_run_http"`
	ContractRunEcosystem string `schema:"contract_run_ecosystem"`
}

func (f *VDEAgentMemberForm) Validate(r *http.Request) error {
	return nil
}

type VDEDestMemberForm struct {
	VDEPubKey            string `schema:"vde_pub_key"`
	VDEComment           string `schema:"vde_comment"`
	VDEName              string `schema:"vde_name"`
	VDEIp                string `schema:"vde_ip"`
	VDEType              int64  `schema:"vde_type"`
	ContractRunHttp      string `schema:"contract_run_http"`
	ContractRunEcosystem string `schema:"contract_run_ecosystem"`
}

func (f *VDEDestMemberForm) Validate(r *http.Request) error {
	return nil
}
