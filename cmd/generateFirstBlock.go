/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package cmd

import (
	"os"
	"path/filepath"
	"time"

	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var stopNetworkBundleFilepath string
var testBlockchain bool
var privateBlockchain bool

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
}

func genesisBlock() ([]byte, error) {
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
	decodeKeyFile := func(kName string) []byte {
		filepath := filepath.Join(conf.Config.DirPathConf.KeysDir, kName)
		data, err := os.ReadFile(filepath)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{"key": kName, "filepath": filepath}).Fatal("Reading key data")
		}

		decodedKey, err := crypto.HexToPub(string(data))
		if err != nil {
			log.WithError(err).Fatalf("converting %s from hex", kName)
		}

		return decodedKey
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
	tx, err := fbp.BinMarshal(&types.FirstBlock{
		KeyID:                 conf.Config.KeyID,
		Time:                  now,
		PublicKey:             decodeKeyFile(consts.PublicKeyFilename),
		NodePublicKey:         decodeKeyFile(consts.NodePublicKeyFilename),
		StopNetworkCertBundle: stopNetworkCert,
		Test:                  test,
		PrivateBlockchain:     pb,
	})
	if err != nil {
		log.WithFields(log.Fields{"type": consts.MarshallingError, "error": err}).Fatal("first block body bin marshalling")
	}
	return block.MarshallBlock(types.WithCurHeader(header),
		types.WithPrevHeader(&types.BlockHeader{
			BlockHash:     crypto.DoubleHash([]byte(`0`)),
			RollbacksHash: crypto.Hash([]byte(`0`)),
		}), types.WithTxFullData([][]byte{tx}))
}
