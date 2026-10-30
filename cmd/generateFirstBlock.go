/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	consensus "github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var stopNetworkBundleFilepath string
var testBlockchain bool
var privateBlockchain bool
var honorNodesOnly bool
var bftConsensus bool
var honorNodesFilepath string
var accountAlgorithms string
var supplyFilepath string

// generateFirstBlockCmd represents the generateFirstBlock command
var generateFirstBlockCmd = &cobra.Command{
	Use:    "generateFirstBlock",
	Short:  "First generation",
	PreRun: loadConfigWKey,
	Run: func(cmd *cobra.Command, args []string) {
		block, err := genesisBlock()
		if err != nil {
			log.WithFields(log.Fields{"type": consts.MarshallingError, "error": err}).Fatal("first block marshalling")
		}
		os.WriteFile(conf.Config.DirPathConf.FirstBlockPath, block, 0644)
		log.Info("first block generated")
	},
}

func init() {
	generateFirstBlockCmd.Flags().StringVar(&stopNetworkBundleFilepath, "stopNetworkCert", "", "Filepath to the fullchain of certificates for network stopping")
	generateFirstBlockCmd.Flags().BoolVar(&testBlockchain, "test", false, "if true - test blockchain")
	generateFirstBlockCmd.Flags().BoolVar(&privateBlockchain, "private", false, "if true - all transactions will be free")
	generateFirstBlockCmd.Flags().BoolVar(&honorNodesOnly, "honorNodesOnly", false, "if true - only the honor nodes generate blocks, the chain never enters the candidate node mode")
	generateFirstBlockCmd.Flags().BoolVar(&bftConsensus, "bftConsensus", false, "if true - the validators commit each block by the votes of more than two thirds of them (needs --honorNodesOnly and --honorNodes)")
	generateFirstBlockCmd.Flags().StringVar(&honorNodesFilepath, "honorNodes", "", "file of the validators of a chain of the BFT consensus, in the JSON of the platform parameter honor_nodes, each with the key_id of its operator; this node first, operated by the founder")
	generateFirstBlockCmd.Flags().StringVar(&supplyFilepath, "supply", "", `file of the supply of the coin of ecosystem 1 and the allocations of the first block, in the smallest unit, e.g. {"supply":"2100000000000000000000","allocations":[{"name":"reserve","account":"1234-5678-9012-3456-7890","amount":"210000000000000000000","conditions":"@1ReserveRelease"}]}: an allocation to an account other than the founder's names the contracts that move it; the rest of the supply is not issued (default: the supply 2100000000, the founder 5250000)`)
	generateFirstBlockCmd.Flags().StringVar(&accountAlgorithms, "accountAlgorithms", "", `algorithms account keys may have, as the platform parameter account_algorithms, e.g. [{"algo":"ECC_P256","sign_until":"2031-12-31"},{"algo":"MLDSA65"}] (default: the node algorithm, with the latest days FIPS allows on a FIPS node and no days otherwise)`)
}

func genesisBlock() ([]byte, error) {
	// The genesis block is signed with the node key like every block (block.MarshallBlock)
	if err := syspar.ReadNodeKeys(); err != nil {
		log.WithError(err).Fatal("reading node keys")
	}
	now := time.Now().Unix()
	header := &types.BlockHeader{
		BlockId:       1,
		Timestamp:     now,
		EcosystemId:   0,
		KeyId:         conf.Config.KeyID,
		NetworkId:     conf.Config.LocalConf.NetworkID,
		NodePosition:  0,
		Version:       consts.BlockVersion,
		RollbacksHash: crypto.Hash([]byte(`0`)),
		ConsensusMode: consts.HonorNodeMode,
	}
	readKeyFile := func(kName string) string {
		filepath := filepath.Join(conf.Config.DirPathConf.KeysDir, kName)
		data, err := os.ReadFile(filepath)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{"key": kName, "filepath": filepath}).Fatal("Reading key data")
		}
		return strings.TrimSpace(string(data))
	}
	nodeKey, err := crypto.HexToPub(readKeyFile(consts.NodePublicKeyFilename))
	if err != nil {
		log.WithError(err).Fatalf("converting %s from hex", consts.NodePublicKeyFilename)
	}
	founderKey, err := crypto.ParseAccountKeyHex(readKeyFile(consts.PublicKeyFilename))
	if err != nil {
		log.WithError(err).Fatalf("reading the account key %s", consts.PublicKeyFilename)
	}
	algos := accountAlgorithms
	if algos == "" {
		algos = syspar.DefaultAccountAlgorithms(crypto.NodeAlgo(), crypto.FIPSMode()).String()
	}

	var stopNetworkCert []byte
	if len(stopNetworkBundleFilepath) > 0 {
		var err error
		fp := filepath.Join(conf.Config.DirPathConf.KeysDir, stopNetworkBundleFilepath)
		if stopNetworkCert, err = os.ReadFile(fp); err != nil {
			log.WithError(err).WithFields(log.Fields{"filepath": fp}).Fatal("Reading cert data")
		}
	}

	if len(stopNetworkCert) == 0 {
		log.Warn("the fullchain of certificates for a network stopping is not specified")
	}

	var test int64
	var pb uint64
	if testBlockchain == true {
		test = 1
	}
	if privateBlockchain == true {
		pb = 1
	}
	var honorOnly uint64
	if honorNodesOnly {
		honorOnly = 1
	}
	var bft uint64
	var honorNodes string
	if len(honorNodesFilepath) > 0 && !bftConsensus {
		log.Fatal("--honorNodes names the validators of a chain of the BFT consensus: it needs --bftConsensus")
	}
	if bftConsensus {
		if crypto.HashSize() != consensus.HashSize {
			log.WithFields(log.Fields{"size": crypto.HashSize()}).Fatal("--bftConsensus needs a hash of 32 bytes: the votes sign the hash of a block")
		}
		bft = 1
		data, err := os.ReadFile(honorNodesFilepath)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{"filepath": honorNodesFilepath}).Fatal("Reading the honor nodes")
		}
		honorNodes = strings.TrimSpace(string(data))
	}

	supply, allocs := readSupply()

	fbp := new(transaction.FirstBlockParser)
	first := &types.FirstBlock{
		KeyID:                 conf.Config.KeyID,
		Time:                  now,
		PublicKey:             founderKey.Bytes(),
		NodePublicKey:         nodeKey,
		StopNetworkCertBundle: stopNetworkCert,
		Test:                  test,
		ChainFormat:           consts.ChainFormat,
		PrivateBlockchain:     pb,
		HonorNodesOnly:        honorOnly,
		AccountAlgorithms:     algos,
		BftConsensus:          bft,
		HonorNodes:            honorNodes,
		Supply:                supply,
		Allocations:           allocs,
	}
	if _, _, err := transaction.CheckFirstBlockAccounts(first); err != nil {
		log.WithError(err).Fatal("checking the first block")
	}
	tx, err := fbp.BinMarshal(first)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.MarshallingError, "error": err}).Fatal("first block body bin marshalling")
	}
	return block.MarshallBlock(types.WithCurHeader(header),
		types.WithPrevHeader(&types.BlockHeader{
			BlockHash:     crypto.DoubleHash([]byte(`0`)),
			RollbacksHash: crypto.Hash([]byte(`0`)),
		}), types.WithTxFullData([][]byte{tx}))
}

// readSupply reads the file of --supply: the supply and the allocations of the first block, each
// to an account of the form xxxx-xxxx-xxxx-xxxx-xxxx or its key_id
func readSupply() (string, []types.Allocation) {
	if supplyFilepath == "" {
		return "", nil
	}
	data, err := os.ReadFile(supplyFilepath)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{"filepath": supplyFilepath}).Fatal("Reading the supply")
	}
	var file struct {
		Supply      string `json:"supply"`
		Allocations []struct {
			Name       string `json:"name"`
			Account    string `json:"account"`
			Amount     string `json:"amount"`
			Conditions string `json:"conditions"`
		} `json:"allocations"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		log.WithError(err).WithFields(log.Fields{"filepath": supplyFilepath}).Fatal("Parsing the supply")
	}
	if file.Supply == "" || len(file.Allocations) == 0 {
		log.WithFields(log.Fields{"filepath": supplyFilepath}).Fatal("--supply names the supply and at least an allocation")
	}
	allocs := make([]types.Allocation, len(file.Allocations))
	for i, a := range file.Allocations {
		id := converter.StringToAddress(a.Account)
		if id == 0 {
			log.WithFields(log.Fields{"allocation": a.Name, "account": a.Account}).Fatal("the account of an allocation is not an account")
		}
		allocs[i] = types.Allocation{Name: a.Name, KeyID: id, Amount: a.Amount, Conditions: a.Conditions}
	}
	return file.Supply, allocs
}
