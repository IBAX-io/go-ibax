/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package chain

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/IBAX-io/go-ibax/packages/api"
	"github.com/IBAX-io/go-ibax/packages/chain/daemonsctl"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/daemons"
	logtools "github.com/IBAX-io/go-ibax/packages/log"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/modes"
	"github.com/IBAX-io/go-ibax/packages/network/httpserver"
	"github.com/IBAX-io/go-ibax/packages/obsmanager"
	"github.com/IBAX-io/go-ibax/packages/publisher"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/statsd"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

func initStatsd() {
	if err := statsd.Init(conf.Config.StatsD); err != nil {
		log.WithFields(log.Fields{"type": consts.StatsdError, "error": err}).Fatal("cannot initialize statsd")
	}
}

func killOld() {
	pidPath := conf.Config.GetPidPath()
	if _, err := os.Stat(pidPath); err == nil {
		dat, err := os.ReadFile(pidPath)
		if err != nil {
			log.WithFields(log.Fields{"path": pidPath, "error": err, "type": consts.IOError}).Error("reading pid file")
		}
		var pidMap map[string]string
		err = json.Unmarshal(dat, &pidMap)
		if err != nil {
			log.WithFields(log.Fields{"data": dat, "error": err, "type": consts.JSONUnmarshallError}).Error("unmarshalling pid map")
		}
		log.WithFields(log.Fields{"path": conf.Config.DirPathConf.DataDir + pidMap["pid"]}).Debug("old pid path")

		KillPid(pidMap["pid"])
		if fmt.Sprintf("%s", err) != "null" {
			// give 15 sec to end the previous process
			for i := 0; i < 5; i++ {
				if _, err := os.Stat(conf.Config.GetPidPath()); err == nil {
					time.Sleep(time.Second)
				} else {
					break
				}
			}
		}
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
		sysLogHook, err := logtools.NewSyslogHook(tag, facility)
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

func savePid() error {
	pid := os.Getpid()
	PidAndVer, err := json.Marshal(map[string]string{"pid": converter.IntToStr(pid), "version": consts.VERSION})
	if err != nil {
		log.WithFields(log.Fields{"pid": pid, "error": err, "type": consts.JSONMarshallError}).Error("marshalling pid to json")
		return err
	}

	return os.WriteFile(conf.Config.GetPidPath(), PidAndVer, 0644)
}

func delPidFile() {
	os.Remove(conf.Config.GetPidPath())
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

// Start starts the main code of the program
func Start() {
	var err error

	defer func() {
		if r := recover(); r != nil {
			log.WithFields(log.Fields{"panic": r, "type": consts.PanicRecoveredError}).Error("recovered panic")
			panic(r)
		}
	}()

	Exit := func(code int) {
		delPidFile()
		model.GormClose()
		statsd.Close()
		os.Exit(code)
	}

	initGorm := func(dbCfg conf.DBConfig) {
		err = model.GormInit(dbCfg)
		if err != nil {
			log.WithFields(log.Fields{
				"db_user": dbCfg.User, "db_password": dbCfg.Password, "db_name": dbCfg.Name, "type": consts.DBError,
			}).Error("can't init gorm")
			Exit(1)
		}
	}

	//log.WithFields(log.Fields{"mode": conf.Config.RunNodeMode}).Info("Node running mode")
	if conf.Config.FuncBench {
		log.Warning("Warning! Access checking is disabled in some built-in functions")
	}

	f := utils.LockOrDie(conf.Config.DirPathConf.LockFilePath)
	defer f.Unlock()
	if err := utils.MakeDirectory(conf.Config.DirPathConf.TempDir); err != nil {
		log.WithFields(log.Fields{"error": err, "type": consts.IOError, "dir": conf.Config.DirPathConf.TempDir}).Error("can't create temporary directory")
		Exit(1)
	}

	initGorm(conf.Config.DB)
	log.WithFields(log.Fields{"work_dir": conf.Config.DirPathConf.DataDir, "version": consts.Version()}).Info("started with")

	killOld()

	publisher.InitCentrifugo(conf.Config.Centrifugo)
	initStatsd()

	err = initLogs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "logs init failed: %v\n", utils.ErrInfo(err))
		Exit(1)
	}

	rand.Seed(time.Now().UTC().UnixNano())

	// save the current pid and version
	if err := savePid(); err != nil {
		log.Errorf("can't create pid: %s", err)
		Exit(1)
	}
	defer delPidFile()

	smart.InitVM()
	if err := syspar.ReadNodeKeys(); err != nil {
		log.Errorf("can't read node keys: %s", err)
		Exit(1)
	}
	if model.DBConn != nil {
		if err := model.UpdateSchema(); err != nil {
			log.WithFields(log.Fields{"error": err}).Error("on running update migrations")
			os.Exit(1)
		}

		ctx, cancel := context.WithCancel(context.Background())
		utils.CancelFunc = cancel
		utils.ReturnCh = make(chan string)

		// The installation process is already finished (where user has specified DB and where wallet has been restarted)
		err := daemonsctl.RunAllDaemons(ctx)
		log.Info("Daemons started")
		if err != nil {
			Exit(1)
		}

		obsmanager.InitOBSManager()
	}
	daemons.WaitForSignals()

	initRoutes(conf.Config.HTTP.Str())

	select {}
}
