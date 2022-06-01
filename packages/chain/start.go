/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package chain

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/IBAX-io/go-ibax/packages/api"
	"github.com/IBAX-io/go-ibax/packages/chain/daemonsctl"
	"github.com/IBAX-io/go-ibax/packages/chain/system"

	logtools "github.com/IBAX-io/go-ibax/packages/common/log"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/daemons"
	"github.com/IBAX-io/go-ibax/packages/modes"
	"github.com/IBAX-io/go-ibax/packages/network/httpserver"
	"github.com/IBAX-io/go-ibax/packages/publisher"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/statsd"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/utils"
	log "github.com/sirupsen/logrus"
)

// Start starts the main code of the program
func Start() {
	var err error
	defer func() {
		if r := recover(); r != nil {
			log.WithFields(log.Fields{"panic": r, "type": consts.PanicRecoveredError}).Error("recovered panic")
			panic(r)
		}
	}()
	exitErr := func(code int) {
		system.RemovePidFile()
		sqldb.GormClose()
		statsd.Close()
		os.Exit(code)
	}
	f := utils.LockOrDie(conf.Config.DirPathConf.LockFilePath)
	defer f.Unlock()
	if err := utils.MakeDirectory(conf.Config.DirPathConf.TempDir); err != nil {
		log.WithFields(log.Fields{"error": err, "type": consts.IOError, "dir": conf.Config.DirPathConf.TempDir}).Error("can't create temporary directory")
		exitErr(1)
	}

	// save the current pid and version
	if err := system.CreatePidFile(); err != nil {
		log.Errorf("can't create pid: %s", err)
		exitErr(1)
	}
	defer system.RemovePidFile()

	log.WithFields(log.Fields{"work_dir": conf.Config.DirPathConf.DataDir, "version": consts.Version()}).Info("started with")

	if err = initLogs(); err != nil {
		log.Errorf("logs init failed: %v\n", utils.ErrInfo(err))
		exitErr(1)
	}

	if conf.Config.FuncBench {
		log.Warning("Warning! Access checking is disabled in some built-in functions")
	}

	publisher.InitCentrifugo(conf.Config.Centrifugo)
	initStatsd()

	rand.Seed(time.Now().UTC().UnixNano())
	smart.InitVM()
	if err := syspar.ReadNodeKeys(); err != nil {
		log.Errorf("can't read node keys: %s", err)
		exitErr(1)
	}

	if err = sqldb.GormInit(conf.Config.DB); err != nil {
		log.WithFields(log.Fields{
			"db_user": conf.Config.DB.User, "db_password": conf.Config.DB.Password, "db_name": conf.Config.DB.Name, "type": consts.DBError,
		}).Error("can't init gorm")
		exitErr(1)
	}

	if sqldb.DBConn != nil {
		if err := sqldb.UpdateSchema(); err != nil {
			log.WithError(err).Error("on running update migrations")
			exitErr(1)
		}
		candidateNodes, err := sqldb.GetCandidateNode(syspar.SysInt(syspar.NumberNodes))
		if err == nil && len(candidateNodes) > 0 {
			syspar.SetRunModel(consts.CandidateNodeMode)
		} else {
			syspar.SetRunModel(consts.HonorNodeMode)
		}

		ctx, cancel := context.WithCancel(context.Background())
		utils.CancelFunc = cancel
		utils.ReturnCh = make(chan string)

		// The installation process is already finished (where user has specified DB and where wallet has been restarted)
		err = daemonsctl.RunAllDaemons(ctx)
		log.Info("Daemons started")
		if err != nil {
			exitErr(1)
		}
	}

	daemons.WaitForSignals()

	initRoutes(conf.Config.HTTP.Str())

	select {}
}

func initStatsd() {
	if err := statsd.Init(conf.Config.StatsD); err != nil {
		log.WithFields(log.Fields{"type": consts.StatsdError, "error": err}).Fatal("cannot initialize statsd")
	}
}

func initLogs() error {
	switch conf.Config.Log.LogFormat {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
	default:
		log.SetFormatter(&log.TextFormatter{})
	}
	switch conf.Config.Log.LogTo {
	case "stdout":
		log.SetOutput(os.Stdout)
	case "syslog":
		facility := conf.Config.Log.Syslog.Facility
		tag := conf.Config.Log.Syslog.Tag
		sysLogHook, err := logtools.NewSyslogHook(facility, tag)
		if err != nil {
			log.WithError(err).Error("Unable to connect to local syslog daemon")
		} else {
			log.AddHook(sysLogHook)
		}
	default:
		fileName := filepath.Join(conf.Config.DirPathConf.DataDir, conf.Config.Log.LogTo)
		openMode := os.O_APPEND
		if _, err := os.Stat(fileName); os.IsNotExist(err) {
			openMode = os.O_CREATE
		}

		f, err := os.OpenFile(fileName, os.O_WRONLY|openMode, 0755)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Can't open log file: ", fileName)
			return err
		}
		log.SetOutput(f)
	}

	switch conf.Config.Log.LogLevel {
	case "DEBUG":
		log.SetLevel(log.DebugLevel)
	case "INFO":
		log.SetLevel(log.InfoLevel)
	case "WARN":
		log.SetLevel(log.WarnLevel)
	case "ERROR":
		log.SetLevel(log.ErrorLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}

	log.AddHook(logtools.ContextHook{})
	log.AddHook(logtools.HexHook{})

	return nil
}

func initRoutes(listenHost string) {
	handler := modes.RegisterRoutes()
	handler = api.WithCors(handler)
	handler = httpserver.NewMaxBodyReader(handler, conf.Config.LocalConf.HTTPServerMaxBodySize)

	if conf.Config.TLSConf.Enabled {
		if len(conf.Config.TLSConf.TLSCert) == 0 || len(conf.Config.TLSConf.TLSKey) == 0 {
			log.Fatal("-tls-cert/TLSCert and -tls-key/TLSKey must be specified with -tls/TLS")
		}
		if _, err := os.Stat(conf.Config.TLSConf.TLSCert); os.IsNotExist(err) {
			log.WithError(err).Fatalf(`Filepath -tls-cert/TLSCert = %s is invalid`, conf.Config.TLSConf.TLSCert)
		}
		if _, err := os.Stat(conf.Config.TLSConf.TLSKey); os.IsNotExist(err) {
			log.WithError(err).Fatalf(`Filepath -tls-key/TLSKey = %s is invalid`, conf.Config.TLSConf.TLSKey)
		}
		go func() {
			s := &http.Server{
				Addr:    listenHost,
				Handler: handler,
				TLSConfig: &tls.Config{
					MinVersion:             tls.VersionTLS12,
					SessionTicketsDisabled: true,
					//ClientAuth:   tls.RequireAndVerifyClientCert,
				},
			}
			err := s.ListenAndServeTLS(conf.Config.TLSConf.TLSCert, conf.Config.TLSConf.TLSKey)

			//err := http.ListenAndServeTLS(listenHost, conf.Config.TLSCert, conf.Config.TLSKey, handler)
			if err != nil {
				log.WithFields(log.Fields{"host": listenHost, "error": err, "type": consts.NetworkError}).Fatal("Listening TLS server")
			}
		}()
		log.WithFields(log.Fields{"host": listenHost}).Info("listening with TLS at")
		return
	}

	httpListener(listenHost, handler)
}
