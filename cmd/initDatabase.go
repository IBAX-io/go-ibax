/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

// initDatabaseCmd represents the initDatabase command
var initDatabaseCmd = &cobra.Command{
	Use:    "initDatabase",
	Short:  "Initializing database",
	PreRun: loadConfigWKey,
	Run: func(cmd *cobra.Command, args []string) {
		if err := sqldb.InitDB(conf.Config.DB); err != nil {
			log.WithError(err).Fatal("init db")
		}
		log.Info("initDatabase completed")
	},
}
