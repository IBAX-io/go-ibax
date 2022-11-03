/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Initial config generation",
	Run: func(cmd *cobra.Command, args []string) {
		// Error omitted because we have default flag value
		configPath, _ := cmd.Flags().GetString("path")

		err := conf.FillRuntimePaths()
		if err != nil {
			log.WithError(err).Fatal("Filling config")
		}

		if configPath == "" {
			configPath = filepath.Join(conf.Config.DirPathConf.DataDir, consts.DefaultConfigFile)
		}
		err = viper.Unmarshal(&conf.Config)
		if err != nil {
			log.WithError(err).Fatal("Marshalling config to global struct variable")
		}

		err = conf.SaveConfig(configPath)
		if err != nil {
			log.WithError(err).Fatal("Saving config")
		}
		log.Infof("Config is saved to %s", configPath)
	},
}

func init() {

	cmdFlags := configCmd.Flags()
	// Command flags
	cmdFlags.String("path", "", "Generate config to (default dataDir/config.toml)")

	// Etc
	cmdFlags.StringVar(&conf.Config.DirPathConf.PidFilePath, "pid", "",
		fmt.Sprintf("ibax pid file name (default dataDir/%s)", consts.DefaultPidFilename),
	)
	cmdFlags.StringVar(&conf.Config.DirPathConf.LockFilePath, "lock", "",
		fmt.Sprintf("ibax lock file name (default dataDir/%s)", consts.DefaultLockFilename),
	)
	cmdFlags.StringVar(&conf.Config.DirPathConf.KeysDir, "keysDir", "", "Keys directory (default dataDir)")
	cmdFlags.StringVar(&conf.Config.DirPathConf.DataDir, "dataDir", "", "Data directory (default cwd/data)")
	cmdFlags.StringVar(&conf.Config.DirPathConf.TempDir, "tempDir", "", "Temporary directory (default temporary directory of OS)")
	cmdFlags.StringVar(&conf.Config.DirPathConf.FirstBlockPath, "firstBlock", "", "First block path (default dataDir/1block)")

	// tls
	cmdFlags.BoolVar(&conf.Config.TLSConf.Enabled, "tlsEnable", false, "Enable https")
	cmdFlags.StringVar(&conf.Config.TLSConf.TLSCert, "tlsCert", "", "Filepath to the fullchain of certificates")
	cmdFlags.StringVar(&conf.Config.TLSConf.TLSKey, "tlsKey", "", "Filepath to the private key")

	//Bootstrap
	cmdFlags.StringSliceVar(&conf.Config.BootNodes.NodesAddr, "bootNodes", []string{}, "List of addresses for downloading blockchain")

	//LocalConf
	cmdFlags.Int64Var(&conf.Config.LocalConf.MaxPageGenerationTime, "mpgt", 3000, "Max page generation time in ms")
	cmdFlags.Int64Var(&conf.Config.LocalConf.HTTPServerMaxBodySize, "mbs", 1<<20, "Max server body size in byte")
	cmdFlags.Int64Var(&conf.Config.LocalConf.NetworkID, "networkID", 1, "Network ID")
	cmdFlags.StringVar(&conf.Config.LocalConf.RunNodeMode, "runMode", consts.NoneCLB, "running node mode, example NONE|CLB|CLBMaster|SubNode")

	// TCP Server
	cmdFlags.StringVar(&conf.Config.TCPServer.Host, "tcpHost", "127.0.0.1", "Node TCP host")
	cmdFlags.IntVar(&conf.Config.TCPServer.Port, "tcpPort", 7078, "Node TCP port")

	// HTTP Server
	cmdFlags.StringVar(&conf.Config.HTTP.Host, "httpHost", "127.0.0.1", "Node HTTP host")
	cmdFlags.IntVar(&conf.Config.HTTP.Port, "httpPort", 7079, "Node HTTP port")

	// DB
	cmdFlags.StringVar(&conf.Config.DB.Host, "dbHost", "127.0.0.1", "DB host")
	cmdFlags.IntVar(&conf.Config.DB.Port, "dbPort", 5432, "DB port")
	cmdFlags.StringVar(&conf.Config.DB.Name, "dbName", "ibax", "DB name")
	cmdFlags.StringVar(&conf.Config.DB.User, "dbUser", "postgres", "DB username")
	cmdFlags.StringVar(&conf.Config.DB.Password, "dbPassword", "123456", "DB password")
	cmdFlags.IntVar(&conf.Config.DB.LockTimeout, "dbLockTimeout", 5000, "DB lock timeout")
	cmdFlags.IntVar(&conf.Config.DB.IdleInTxTimeout, "dbIdleInTxTimeout", 5000, "DB idle tx timeout")
	cmdFlags.IntVar(&conf.Config.DB.MaxIdleConns, "dbMaxIdleConns", 5, "DB sets the maximum number of connections in the idle connection pool")
	cmdFlags.IntVar(&conf.Config.DB.MaxOpenConns, "dbMaxOpenConns", 100, "sets the maximum number of open connections to the database")

	//Redis
	cmdFlags.BoolVar(&conf.Config.Redis.Enable, "redisEnable", false, "enable redis")
	cmdFlags.StringVar(&conf.Config.Redis.Host, "redisHost", "localhost", "redis host")
	cmdFlags.IntVar(&conf.Config.Redis.Port, "redisPort", 6379, "redis port")
	cmdFlags.IntVar(&conf.Config.Redis.DbName, "redisDb", 0, "redis db")
	cmdFlags.StringVar(&conf.Config.Redis.Password, "redisPassword", "123456", "redis password")

	// StatsD
	cmdFlags.StringVar(&conf.Config.StatsD.Host, "statsdHost", "127.0.0.1", "StatsD host")
	cmdFlags.IntVar(&conf.Config.StatsD.Port, "statsdPort", 8125, "StatsD port")
	cmdFlags.StringVar(&conf.Config.StatsD.Name, "statsdName", "chain", "StatsD name")

	// Centrifugo
	cmdFlags.StringVar(&conf.Config.Centrifugo.Secret, "centSecret", "127.0.0.1", "Centrifugo secret")
	cmdFlags.StringVar(&conf.Config.Centrifugo.URL, "centUrl", "127.0.0.1", "Centrifugo URL")
	cmdFlags.StringVar(&conf.Config.Centrifugo.Key, "centKey", "127.0.0.1", "Centrifugo API key")

	// Log
	cmdFlags.StringVar(&conf.Config.Log.LogTo, "logTo", "stdout", "Send logs to stdout|(filename)|syslog")
	cmdFlags.StringVar(&conf.Config.Log.LogLevel, "logLevel", "ERROR", "Log verbosity (DEBUG | INFO | WARN | ERROR)")
	cmdFlags.StringVar(&conf.Config.Log.LogFormat, "logFormat", "text", "log format, could be text|json")
	cmdFlags.StringVar(&conf.Config.Log.Syslog.Facility, "syslogFacility", "kern", "syslog facility")
	cmdFlags.StringVar(&conf.Config.Log.Syslog.Tag, "syslogTag", "go-ibax", "syslog program tag")

	// TokenMovement
	cmdFlags.StringVar(&conf.Config.TokenMovement.Host, "tmovHost", "", "Token movement host")
	cmdFlags.IntVar(&conf.Config.TokenMovement.Port, "tmovPort", 0, "Token movement port")
	cmdFlags.StringVar(&conf.Config.TokenMovement.Username, "tmovUser", "", "Token movement username")
	cmdFlags.StringVar(&conf.Config.TokenMovement.Password, "tmovPw", "", "Token movement password")
	cmdFlags.StringVar(&conf.Config.TokenMovement.To, "tmovTo", "", "Token movement to field")
	cmdFlags.StringVar(&conf.Config.TokenMovement.From, "tmovFrom", "", "Token movement from field")
	cmdFlags.StringVar(&conf.Config.TokenMovement.Subject, "tmovSubj", "", "Token movement subject")

	cmdFlags.IntVar(&conf.Config.BanKey.BadTime, "badTime", 5, "Period for bad tx (minutes)")
	cmdFlags.IntVar(&conf.Config.BanKey.BanTime, "banTime", 15, "Ban time in minutes")
	cmdFlags.IntVar(&conf.Config.BanKey.BadTx, "badTx", 5, "Maximum bad tx during badTime minutes")

	// CryptoSettings
	cmdFlags.StringVar(&conf.Config.CryptoSettings.Hasher, "hasher", crypto.HashAlgo_KECCAK256.String(), fmt.Sprintf("Hash Algorithm (%s | %s | %s | %s)", crypto.HashAlgo_SHA256, crypto.HashAlgo_KECCAK256, crypto.HashAlgo_SHA3_256, crypto.HashAlgo_SM3))
	cmdFlags.StringVar(&conf.Config.CryptoSettings.Cryptoer, "cryptoer", crypto.AsymAlgo_ECC_Secp256k1.String(), fmt.Sprintf("Key and Sign Algorithm (%s | %s | %s | %s)", crypto.AsymAlgo_ECC_P256, crypto.AsymAlgo_ECC_Secp256k1, crypto.AsymAlgo_ECC_P512, crypto.AsymAlgo_SM2))

	// BlockSyncMethod
	cmdFlags.StringVar(&conf.Config.BlockSyncMethod.Method, "sync", types.BlockSyncMethod_CONTRACTVM.String(), fmt.Sprintf("Block sync method (%s | %s)", types.BlockSyncMethod_CONTRACTVM, types.BlockSyncMethod_SQLDML))

	viper.BindPFlags(configCmd.PersistentFlags())
}
