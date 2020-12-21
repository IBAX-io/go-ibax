/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daylight

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
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/daemons"
	"github.com/IBAX-io/go-ibax/packages/daylight/daemonsctl"
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
	cfg := conf.Config.StatsD
	if err := statsd.Init(cfg.Host, cfg.Port, cfg.Name); err != nil {
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
		log.WithFields(log.Fields{"path": conf.Config.DataDir + pidMap["pid"]}).Debug("old pid path")

		KillPid(pidMap["pid"])
		if fmt.Sprintf("%s", err) != "null" {
			// give 15 sec to end the previous process
			for i := 0; i < 10; i++ {
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
			log.WithError(err).Error("initializing syslog hook")
		} else {
			log.AddHook(sysLogHook)
		}
	default:
		fileName := filepath.Join(conf.Config.DataDir, conf.Config.Log.LogTo)
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
	handler = httpserver.NewMaxBodyReader(handler, conf.Config.HTTPServerMaxBodySize)

	if conf.Config.TLS {
		if len(conf.Config.TLSCert) == 0 || len(conf.Config.TLSKey) == 0 {
			log.Fatal("-tls-cert/TLSCert and -tls-key/TLSKey must be specified with -tls/TLS")
		}
		if _, err := os.Stat(conf.Config.TLSCert); os.IsNotExist(err) {
			log.WithError(err).Fatalf(`Filepath -tls-cert/TLSCert = %s is invalid`, conf.Config.TLSCert)
		}
		if _, err := os.Stat(conf.Config.TLSKey); os.IsNotExist(err) {
			log.WithError(err).Fatalf(`Filepath -tls-key/TLSKey = %s is invalid`, conf.Config.TLSKey)
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
			err := s.ListenAndServeTLS(conf.Config.TLSCert, conf.Config.TLSKey)

			//err := http.ListenAndServeTLS(listenHost, conf.Config.TLSCert, conf.Config.TLSKey, handler)
			if err != nil {
				log.WithFields(log.Fields{"host": listenHost, "error": err, "type": consts.NetworkError}).Fatal("Listening TLS server")
			}
		}()
		log.WithFields(log.Fields{"host": listenHost}).Info("listening with TLS at")
		return
	} else if len(conf.Config.TLSCert) != 0 || len(conf.Config.TLSKey) != 0 {
		log.Fatal("-tls/TLS must be specified with -tls-cert/TLSCert and -tls-key/TLSKey")
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
		err = model.GormInit(dbCfg.Host, dbCfg.Port, dbCfg.User, dbCfg.Password, dbCfg.Name)
		if err != nil {
		}
	}

	//log.WithFields(log.Fields{"mode": conf.Config.OBSMode}).Info("Node running mode")
	if conf.Config.FuncBench {
		log.Warning("Warning! Access checking is disabled in some built-in functions")
	}

	f := utils.LockOrDie(conf.Config.LockFilePath)
	defer f.Unlock()
	if err := utils.MakeDirectory(conf.Config.TempDir); err != nil {
		log.WithFields(log.Fields{"error": err, "type": consts.IOError, "dir": conf.Config.TempDir}).Error("can't create temporary directory")
		Exit(1)
	}

	if conf.Config.PoolPub.Enable {
		if conf.Config.Redis.Enable {
			err = model.RedisInit(conf.Config.Redis.Host, conf.Config.Redis.Port, conf.Config.Redis.Password, conf.Config.Redis.DbName)
			if err != nil {
				log.WithFields(log.Fields{
					"host": conf.Config.Redis.Host, "port": conf.Config.Redis.Port, "db_password": conf.Config.Redis.Password, "db_name": conf.Config.Redis.DbName, "type": consts.DBError,
				}).Error("can't init redis")
				Exit(1)
			}
		}
		//if err := utils.MakeDirectory(conf.Config.PoolPub.Path); err != nil {
		//	log.WithFields(log.Fields{"error": err, "type": consts.IOError, "dir": conf.Config.TempDir}).Error("can't create temporary directory")
		//	os.Exit(-1)
		//}
		//if err := model.Init_leveldb(conf.Config.PoolPub.Path); err != nil {
		//	log.WithFields(log.Fields{"error": err, "type": consts.IOError, "dir": conf.Config.TempDir}).Error("can't create temporary directory")
		//	os.Exit(-1)
		//}
	}

	initGorm(conf.Config.DB)
	log.WithFields(log.Fields{"work_dir": conf.Config.DataDir, "version": consts.Version()}).Info("started with")

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
