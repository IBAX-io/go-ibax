/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package cmd

import (
	"os"
	"path/filepath"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	addrsForStopping        []string
	stopNetworkCertFilepath string
)

// stopNetworkCmd represents the stopNetworkCmd command
var stopNetworkCmd = &cobra.Command{
	Use:    "stopNetwork",
	Short:  "Sending a special transaction to stop the network",
	PreRun: loadConfigWKey,
	Run: func(cmd *cobra.Command, args []string) {
		fp := filepath.Join(conf.Config.DirPathConf.KeysDir, stopNetworkCertFilepath)
		stopNetworkCert, err := os.ReadFile(fp)
		if err != nil {
			log.WithFields(log.Fields{"error": err, "type": consts.IOError, "filepath": fp}).Fatal("Reading cert data")
		}

		req := &network.StopNetworkRequest{
			Data: stopNetworkCert,
		}

		errCount := 0
		for _, addr := range addrsForStopping {
			if err := tcpclient.SendStopNetwork(addr, req); err != nil {
				log.WithFields(log.Fields{"error": err, "type": consts.NetworkError, "addr": addr}).Errorf("Sending request")
				errCount++
				continue
			}

			log.WithFields(log.Fields{"addr": addr}).Info("Sending request")
		}

		log.WithFields(log.Fields{
			"successful": len(addrsForStopping) - errCount,
			"failed":     errCount,
		}).Info("Complete")
	},
}

func init() {
	stopNetworkCmd.Flags().StringVar(&stopNetworkCertFilepath, "stopNetworkCert", "", "Filepath to certificate for network stopping")
	stopNetworkCmd.Flags().StringArrayVar(&addrsForStopping, "addr", []string{}, "Node address")
	stopNetworkCmd.MarkFlagRequired("stopNetworkCert")
	stopNetworkCmd.MarkFlagRequired("addr")
}
