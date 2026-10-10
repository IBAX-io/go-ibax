/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var stopNetworkBundleFilepath string
var testBlockchain bool
var privateBlockchain bool
var accountAlgorithms string

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
	generateFirstBlockCmd.Flags().StringVar(&accountAlgorithms, "accountAlgorithms", "", `algorithms account keys may have, as the platform parameter account_algorithms, e.g. [{"algo":"ECC_P256","sign_until":"2031-12-31"},{"algo":"MLDSA65"}] (default: the node algorithm, without days)`)
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
		algos = syspar.AccountAlgorithmSet{{Algo: crypto.NodeAlgo()}}.String()
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

	fbp := new(transaction.FirstBlockParser)
	first := &types.FirstBlock{
		KeyID:                 conf.Config.KeyID,
		Time:                  now,
		PublicKey:             founderKey.Bytes(),
		NodePublicKey:         nodeKey,
		StopNetworkCertBundle: stopNetworkCert,
		Test:                  test,
		PrivateBlockchain:     pb,
		AccountAlgorithms:     algos,
	}
	if _, _, err := transaction.CheckFirstBlockAccounts(first); err != nil {
		log.WithError(err).Fatal("checking the accounts of the first block")
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
