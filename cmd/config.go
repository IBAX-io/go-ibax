/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

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
			configPath = filepath.Join(conf.Config.DataDir, consts.DefaultConfigFile)
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
	viper.SetEnvPrefix("CHAIN")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Command flags
	configCmd.Flags().String("path", "", "Generate config to (default dataDir/config.toml)")

	// TCP Server
	configCmd.Flags().StringVar(&conf.Config.TCPServer.Host, "tcpHost", "127.0.0.1", "Node TCP host")
	configCmd.Flags().IntVar(&conf.Config.TCPServer.Port, "tcpPort", 7078, "Node TCP port")
	viper.BindPFlag("TCPServer.Host", configCmd.Flags().Lookup("tcpHost"))
	viper.BindPFlag("TCPServer.Port", configCmd.Flags().Lookup("tcpPort"))

	// HTTP Server
	configCmd.Flags().StringVar(&conf.Config.HTTP.Host, "httpHost", "127.0.0.1", "Node HTTP host")
	configCmd.Flags().IntVar(&conf.Config.HTTP.Port, "httpPort", 7079, "Node HTTP port")
	viper.BindPFlag("HTTP.Host", configCmd.Flags().Lookup("httpHost"))
	viper.BindPFlag("HTTP.Port", configCmd.Flags().Lookup("httpPort"))

	// DB
	configCmd.Flags().StringVar(&conf.Config.DB.Host, "dbHost", "127.0.0.1", "DB host")
	configCmd.Flags().IntVar(&conf.Config.DB.Port, "dbPort", 5432, "DB port")
	configCmd.Flags().StringVar(&conf.Config.DB.Name, "dbName", "ibax", "DB name")
	configCmd.Flags().StringVar(&conf.Config.DB.User, "dbUser", "postgres", "DB username")
	configCmd.Flags().StringVar(&conf.Config.DB.Password, "dbPassword", "123456", "DB password")
	configCmd.Flags().IntVar(&conf.Config.DB.LockTimeout, "dbLockTimeout", 5000, "DB lock timeout")
	configCmd.Flags().IntVar(&conf.Config.DB.IdleInTxTimeout, "dbIdleInTxTimeout", 5000, "DB idle tx timeout")
	configCmd.Flags().IntVar(&conf.Config.DB.MaxIdleConns, "dbMaxIdleConns", 5, "DB sets the maximum number of connections in the idle connection pool")
	configCmd.Flags().IntVar(&conf.Config.DB.MaxOpenConns, "dbMaxOpenConns", 100, "sets the maximum number of open connections to the database")
	viper.BindPFlag("DB.Name", configCmd.Flags().Lookup("dbName"))
	viper.BindPFlag("DB.Host", configCmd.Flags().Lookup("dbHost"))
	viper.BindPFlag("DB.Port", configCmd.Flags().Lookup("dbPort"))
	viper.BindPFlag("DB.User", configCmd.Flags().Lookup("dbUser"))
	viper.BindPFlag("DB.Password", configCmd.Flags().Lookup("dbPassword"))
	viper.BindPFlag("DB.LockTimeout", configCmd.Flags().Lookup("dbLockTimeout"))
	viper.BindPFlag("DB.IdleInTxTimeout", configCmd.Flags().Lookup("dbIdleInTxTimeout"))
	viper.BindPFlag("DB.MaxIdleConns", configCmd.Flags().Lookup("dbMaxIdleConns"))
	viper.BindPFlag("DB.MaxOpenConns", configCmd.Flags().Lookup("dbMaxOpenConns"))

	//Redis
	configCmd.Flags().BoolVar(&conf.Config.Redis.Enable, "redisenable", false, "enable redis")
	configCmd.Flags().StringVar(&conf.Config.Redis.Host, "redishost", "localhost", "redis host")
	configCmd.Flags().StringVar(&conf.Config.Redis.Port, "redisport", "6379", "redis port")
	configCmd.Flags().IntVar(&conf.Config.Redis.DbName, "redisdb", 0, "redis db")
	configCmd.Flags().StringVar(&conf.Config.Redis.Password, "redispassword", "123456", "redis password")

	viper.BindPFlag("Redis.Enable", configCmd.Flags().Lookup("redisenable"))
	viper.BindPFlag("Redis.Host", configCmd.Flags().Lookup("redishost"))
	viper.BindPFlag("Redis.Port", configCmd.Flags().Lookup("redisport"))
	viper.BindPFlag("Redis.Dbname", configCmd.Flags().Lookup("redisdb"))
	viper.BindPFlag("Redis.Password", configCmd.Flags().Lookup("redispassword"))
	// StatsD
	configCmd.Flags().StringVar(&conf.Config.StatsD.Host, "statsdHost", "127.0.0.1", "StatsD host")
	configCmd.Flags().IntVar(&conf.Config.StatsD.Port, "statsdPort", 8125, "StatsD port")
	configCmd.Flags().StringVar(&conf.Config.StatsD.Name, "statsdName", "chain", "StatsD name")
	viper.BindPFlag("StatsD.Host", configCmd.Flags().Lookup("statsdHost"))
	viper.BindPFlag("StatsD.Port", configCmd.Flags().Lookup("statsdPort"))
	viper.BindPFlag("StatsD.Name", configCmd.Flags().Lookup("statsdName"))

	// Centrifugo
	configCmd.Flags().StringVar(&conf.Config.Centrifugo.Secret, "centSecret", "127.0.0.1", "Centrifugo secret")
	configCmd.Flags().StringVar(&conf.Config.Centrifugo.URL, "centUrl", "127.0.0.1", "Centrifugo URL")
	configCmd.Flags().StringVar(&conf.Config.Centrifugo.Key, "centKey", "127.0.0.1", "Centrifugo API key")
	viper.BindPFlag("Centrifugo.Secret", configCmd.Flags().Lookup("centSecret"))
	viper.BindPFlag("Centrifugo.URL", configCmd.Flags().Lookup("centUrl"))
	viper.BindPFlag("Centrifugo.Key", configCmd.Flags().Lookup("centKey"))

	// Log
	configCmd.Flags().StringVar(&conf.Config.Log.LogTo, "logTo", "stdout", "Send logs to stdout|(filename)|syslog")
	configCmd.Flags().StringVar(&conf.Config.Log.LogLevel, "logLevel", "ERROR", "Log verbosity (DEBUG | INFO | WARN | ERROR)")
	configCmd.Flags().StringVar(&conf.Config.Log.LogFormat, "logFormat", "text", "log format, could be text|json")
	configCmd.Flags().StringVar(&conf.Config.Log.Syslog.Facility, "syslogFacility", "kern", "syslog facility")
	// TokenMovement
	configCmd.Flags().StringVar(&conf.Config.TokenMovement.Host, "tmovHost", "", "Token movement host")
	configCmd.Flags().IntVar(&conf.Config.TokenMovement.Port, "tmovPort", 0, "Token movement port")
	configCmd.Flags().StringVar(&conf.Config.TokenMovement.Username, "tmovUser", "", "Token movement username")
	configCmd.Flags().StringVar(&conf.Config.TokenMovement.Password, "tmovPw", "", "Token movement password")
	configCmd.Flags().StringVar(&conf.Config.TokenMovement.To, "tmovTo", "", "Token movement to field")
	configCmd.Flags().StringVar(&conf.Config.TokenMovement.From, "tmovFrom", "", "Token movement from field")
	configCmd.Flags().StringVar(&conf.Config.TokenMovement.Subject, "tmovSubj", "", "Token movement subject")
	viper.BindPFlag("TokenMovement.Host", configCmd.Flags().Lookup("tmovHost"))
	viper.BindPFlag("TokenMovement.Port", configCmd.Flags().Lookup("tmovPort"))
	viper.BindPFlag("TokenMovement.Username", configCmd.Flags().Lookup("tmovUser"))
	viper.BindPFlag("TokenMovement.Password", configCmd.Flags().Lookup("tmovPw"))
	viper.BindPFlag("TokenMovement.To", configCmd.Flags().Lookup("tmovTo"))
	viper.BindPFlag("TokenMovement.From", configCmd.Flags().Lookup("tmovFrom"))
	viper.BindPFlag("TokenMovement.Subject", configCmd.Flags().Lookup("tmovSubj"))

	configCmd.Flags().IntVar(&conf.Config.BanKey.BadTime, "badTime", 5, "Period for bad tx (minutes)")
	configCmd.Flags().IntVar(&conf.Config.BanKey.BanTime, "banTime", 15, "Ban time in minutes")
	configCmd.Flags().IntVar(&conf.Config.BanKey.BadTx, "badTx", 3, "Maximum bad tx during badTime minutes")
	viper.BindPFlag("BanKey.BadTime", configCmd.Flags().Lookup("badTime"))
	viper.BindPFlag("BanKey.BanTime", configCmd.Flags().Lookup("banTime"))
	viper.BindPFlag("BanKey.BadTx", configCmd.Flags().Lookup("badTx"))

	// Etc
	configCmd.Flags().StringVar(&conf.Config.PidFilePath, "pid", "",
		fmt.Sprintf("ibax pid file name (default dataDir/%s)", consts.DefaultPidFilename),
	)
	configCmd.Flags().StringVar(&conf.Config.LockFilePath, "lock", "",
		fmt.Sprintf("ibax lock file name (default dataDir/%s)", consts.DefaultLockFilename),
	)
	configCmd.Flags().StringVar(&conf.Config.KeysDir, "keysDir", "", "Keys directory (default dataDir)")
	configCmd.Flags().StringVar(&conf.Config.DataDir, "dataDir", "", "Data directory (default cwd/data)")
	configCmd.Flags().StringVar(&conf.Config.TempDir, "tempDir", "", "Temporary directory (default temporary directory of OS)")
	configCmd.Flags().StringVar(&conf.Config.FirstBlockPath, "firstBlock", "", "First block path (default dataDir/1block)")
	configCmd.Flags().BoolVar(&conf.Config.TLS, "tls", false, "Enable https")
	configCmd.Flags().StringVar(&conf.Config.TLSCert, "tls-cert", "", "Filepath to the fullchain of certificates")
	configCmd.Flags().StringVar(&conf.Config.TLSKey, "tls-key", "", "Filepath to the private key")
	configCmd.Flags().Int64Var(&conf.Config.MaxPageGenerationTime, "mpgt", 3000, "Max page generation time in ms")
	configCmd.Flags().Int64Var(&conf.Config.HTTPServerMaxBodySize, "mbs", 1<<20, "Max server body size in byte")
	configCmd.Flags().StringSliceVar(&conf.Config.NodesAddr, "nodesAddr", []string{}, "List of addresses for downloading blockchain")
	configCmd.Flags().Int64Var(&conf.Config.NetworkID, "networkID", 1, "Network ID")
	configCmd.Flags().StringVar(&conf.Config.OBSMode, "obsMode", consts.NoneOBS, "OBS running mode")

	viper.BindPFlag("PidFilePath", configCmd.Flags().Lookup("pid"))
	viper.BindPFlag("LockFilePath", configCmd.Flags().Lookup("lock"))
	viper.BindPFlag("KeysDir", configCmd.Flags().Lookup("keysDir"))
	viper.BindPFlag("DataDir", configCmd.Flags().Lookup("dataDir"))
	viper.BindPFlag("FirstBlockPath", configCmd.Flags().Lookup("firstBlock"))
	viper.BindPFlag("TLS", configCmd.Flags().Lookup("tls"))
	viper.BindPFlag("TLSCert", configCmd.Flags().Lookup("tls-cert"))
	viper.BindPFlag("TLSKey", configCmd.Flags().Lookup("tls-key"))
	viper.BindPFlag("MaxPageGenerationTime", configCmd.Flags().Lookup("mpgt"))
	viper.BindPFlag("HTTPServerMaxBodySize", configCmd.Flags().Lookup("mbs"))
	viper.BindPFlag("TempDir", configCmd.Flags().Lookup("tempDir"))
	viper.BindPFlag("NodesAddr", configCmd.Flags().Lookup("nodesAddr"))
	viper.BindPFlag("NetworkID", configCmd.Flags().Lookup("networkID"))
	viper.BindPFlag("OBSMode", configCmd.Flags().Lookup("obsMode"))

	// GFiles
	configCmd.Flags().BoolVar(&conf.Config.GFiles.GFiles, "gfs", false, "Enable GFiles")
	configCmd.Flags().StringVar(&conf.Config.GFiles.Host, "gFilesHost", "127.0.0.1:5001", "GFiles host")
	viper.BindPFlag("GFiles.GFiles", configCmd.Flags().Lookup("gfs"))
	viper.BindPFlag("GFiles.Host", configCmd.Flags().Lookup("gFilesHost"))

	// PoolManage
	configCmd.Flags().BoolVar(&conf.Config.PoolPub.Enable, "poolenable", false, "Enable Pool")
	configCmd.Flags().BoolVar(&conf.Config.PoolPub.MinersCount, "minerscount", false, "Enable MinersCount")
	configCmd.Flags().BoolVar(&conf.Config.PoolPub.MininerMap, "mininermap", false, "Enable MininerMap")
	configCmd.Flags().BoolVar(&conf.Config.PoolPub.TotalCount, "totalcount", false, "Enable TotalCount")
	configCmd.Flags().BoolVar(&conf.Config.PoolPub.RollBack, "rollback", false, "Enable RollBack")
	configCmd.Flags().StringVar(&conf.Config.PoolPub.Path, "poolpath", "leveldb", "leveldb path")
	viper.BindPFlag("PoolPub.Enable", configCmd.Flags().Lookup("poolenable"))
	viper.BindPFlag("PoolPub.MinersCount", configCmd.Flags().Lookup("minerscount"))
	viper.BindPFlag("PoolPub.MininerMap", configCmd.Flags().Lookup("mininermap"))
	viper.BindPFlag("PoolPub.TotalCount", configCmd.Flags().Lookup("totalcount"))
	viper.BindPFlag("PoolPub.RollBack", configCmd.Flags().Lookup("rollback"))
	viper.BindPFlag("PoolPub.Path", configCmd.Flags().Lookup("poolpath"))
	// CryptoSettings
	configCmd.Flags().StringVar(&conf.Config.CryptoSettings.Hasher, "hasher", "SHA256", "Hash Algorithm")
	configCmd.Flags().StringVar(&conf.Config.CryptoSettings.Cryptoer, "cryptoer", "ECDSA", "Key and Sign Algorithm")
	viper.BindPFlag("CryptoSettings.Hasher", configCmd.Flags().Lookup("hasher"))
	viper.BindPFlag("CryptoSettings.Cryptoer", configCmd.Flags().Lookup("cryptoer"))
}
