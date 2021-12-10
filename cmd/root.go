/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package cmd

import (
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/consts"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"path/filepath"

	"github.com/IBAX-io/go-ibax/packages/conf"
)

var (
	buildBranch = ""
	buildDate   = ""
	commitHash  = ""
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "go-ibax",
	Short: "ibax application",
}

func init() {
	rootCmd.AddCommand(
		generateFirstBlockCmd,
		generateKeysCmd,
		initDatabaseCmd,
		rollbackCmd,
		startCmd,
		configCmd,
		stopNetworkCmd,
		versionCmd,
	)

	consts.BuildInfo = func() string {
		if buildBranch == "" {
			return fmt.Sprintf("branch.%s commit.%s time.%s", "unknown", "unknown", "unknown")
		}
		return fmt.Sprintf("branch.%s commit.%s time.%s", buildBranch, commitHash, buildDate)
	}()
	// This flags are visible for all child commands
	rootCmd.PersistentFlags().StringVar(&conf.Config.ConfigPath, "config", defautConfigPath(), "filepath to config.toml")
}

// Execute executes rootCmd command.
// This is called by main.main(). It only needs to happen once to the rootCmd
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.WithError(err).Fatal("Executing root command")
	}
}

func defautConfigPath() string {
	//p, err := os.Getwd()
	//if err != nil {
	//	log.WithError(err).Fatal("getting cur wd")
	//}
	//
	//return filepath.Join(p, "data", "config.toml")
	return filepath.Join("data", "config.toml")
}

// Load the configuration from file
func loadConfig(cmd *cobra.Command, args []string) {
	err := conf.LoadConfig(conf.Config.ConfigPath)
	if err != nil {
		log.WithError(err).Fatal("Loading config")
	}
}

func loadConfigWKey(cmd *cobra.Command, args []string) {
	loadConfig(cmd, args)
	err := conf.FillRuntimeKey()
	if err != nil {
		log.WithError(err).Fatal("Filling keys")
	}
}
