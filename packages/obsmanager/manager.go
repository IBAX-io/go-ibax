/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package obsmanager

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/IBAX-io/go-ibax/packages/conf"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/ochinchina/go-ini"
	pConf "github.com/ochinchina/supervisord/config"
	"github.com/ochinchina/supervisord/process"
	log "github.com/sirupsen/logrus"
)

const (
	childFolder        = "configs"
	createRoleTemplate = `CREATE ROLE %s WITH ENCRYPTED PASSWORD '%s' NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT LOGIN`
	createDBTemplate   = `CREATE DATABASE %s OWNER %s`

	dropDBTemplate     = `DROP DATABASE IF EXISTS %s`
	dropOwnedTemplate  = `DROP OWNED BY %s CASCADE`
	dropDBRoleTemplate = `DROP ROLE IF EXISTS %s`
	commandTemplate    = `%s start --config=%s`

	alreadyExistsErrorTemplate = `obs '%s' already exists`
)

var (
	errWrongMode        = errors.New("node must be running as OBSMaster")
	errIncorrectOBSName = errors.New("the name cannot begit with a number and must contain alphabetical symbols and numbers")
)

// OBSManager struct
type OBSManager struct {
	processes        *process.Manager
	execPath         string
	childConfigsPath string
}

var (
	Manager *OBSManager
)

func prepareWorkDir() (string, error) {
	childConfigsPath := path.Join(conf.Config.DataDir, childFolder)

	if _, err := os.Stat(childConfigsPath); os.IsNotExist(err) {
		if err := os.Mkdir(childConfigsPath, 0700); err != nil {
			log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("creating configs directory")
			return "", err
		}
	}

	return childConfigsPath, nil
}

// CreateOBS creates one instance of OBS
func (mgr *OBSManager) CreateOBS(name, dbUser, dbPassword string, port int) error {
	if err := checkOBSName(name); err != nil {
		log.WithFields(log.Fields{"type": consts.OBSManagerError, "error": err}).Error("on check OBS name")
		return errIncorrectOBSName
	}

	var err error
	var cancelChain []func()

	defer func() {
		if err == nil {
			return
		}

		for _, cancelFunc := range cancelChain {
			cancelFunc()
		}
	}()

	config := ChildOBSConfig{
		Executable:     mgr.execPath,
		Name:           name,
		Directory:      path.Join(mgr.childConfigsPath, name),
		DBUser:         dbUser,
		DBPassword:     dbPassword,
		ConfigFileName: consts.DefaultConfigFile,
		HTTPPort:       port,
		LogTo:          fmt.Sprintf("%s_%s", name, conf.Config.Log.LogTo),
		LogLevel:       conf.Config.Log.LogLevel,
	}

	if mgr.processes == nil {
		log.WithFields(log.Fields{"type": consts.WrongModeError, "error": errWrongMode}).Error("creating new OBS")
		return errWrongMode
	}

	if err = mgr.createOBSDB(name, dbUser, dbPassword); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on creating OBS DB")
		return fmt.Errorf(alreadyExistsErrorTemplate, name)
	}

	cancelChain = append(cancelChain, func() {
		dropDb(name, dbUser)
	})

	dirPath := path.Join(mgr.childConfigsPath, name)
	if directoryExists(dirPath) {
		err = fmt.Errorf(alreadyExistsErrorTemplate, name)
		log.WithFields(log.Fields{"type": consts.OBSManagerError, "error": err, "dirPath": dirPath}).Error("on check directory")
		return err
	}

	if err = mgr.initOBSDir(name); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "DirName": name, "error": err}).Error("on init OBS dir")
		return err
	}

	cancelChain = append(cancelChain, func() {
		dropOBSDir(mgr.childConfigsPath, name)
	})

	cmd := config.configCommand()
	if err = cmd.Run(); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "args": cmd.Args, "error": err}).Error("on run config command")
		return err
	}

	if err = config.generateKeysCommand().Run(); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "args": cmd.Args, "error": err}).Error("on run generateKeys command")
		return err
	}

	if err = config.initDBCommand().Run(); err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "args": cmd.Args}).Error("on run initDB command")
		return err
	}

	procConfEntry := pConf.NewEntry(config.Directory)
	procConfEntry.Name = "program:" + name
	command := fmt.Sprintf("%s start --config=%s", config.Executable, filepath.Join(config.Directory, consts.DefaultConfigFile))
	log.Infoln(command)
	section := ini.NewSection(procConfEntry.Name)
	section.Add("command", command)
	proc := process.NewProcess("obsMaster", procConfEntry)

	mgr.processes.Add(name, proc)
	mgr.processes.Find(name).Start(true)
	return nil
}

// ListProcess returns list of process names with state of process
func (mgr *OBSManager) ListProcess() (map[string]string, error) {
	if mgr.processes == nil {
		log.WithFields(log.Fields{"type": consts.WrongModeError, "error": errWrongMode}).Error("get OBS list")
		return nil, errWrongMode
	}

	list := make(map[string]string)

	mgr.processes.ForEachProcess(func(p *process.Process) {
		list[p.GetName()] = p.GetState().String()
	})

	return list, nil
}

func (mgr *OBSManager) ListProcessWithPorts() (map[string]string, error) {
	list, err := mgr.ListProcess()
	if err != nil {
		return list, err
	}

	for name, status := range list {
		path := path.Join(mgr.childConfigsPath, name, consts.DefaultConfigFile)
		c := &conf.GlobalConfig{}
		if err := conf.LoadConfigToVar(path, c); err != nil {
			log.WithFields(log.Fields{"type": "dbError", "error": err, "path": path}).Warn("on loading child OBS config")
			continue
		}

		list[name] = fmt.Sprintf("%s %d", status, c.HTTP.Port)
	}

	return list, err
}

// DeleteOBS stop OBS process and remove OBS folder
func (mgr *OBSManager) DeleteOBS(name string) error {

	if mgr.processes == nil {
		log.WithFields(log.Fields{"type": consts.WrongModeError, "error": errWrongMode}).Error("deleting OBS")
		return errWrongMode
	}

	mgr.StopOBS(name)
	mgr.processes.Remove(name)
	obsDir := path.Join(mgr.childConfigsPath, name)
	obsConfigPath := filepath.Join(obsDir, consts.DefaultConfigFile)
	obsConfig, err := conf.GetConfigFromPath(obsConfigPath)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Errorf("Getting config from path %s", obsConfigPath)
		return fmt.Errorf(`OBS '%s' is not exists`, name)
	}

	time.Sleep(1 * time.Second)
	if err := dropDb(obsConfig.DB.Name, obsConfig.DB.User); err != nil {
		return err
	}

	return os.RemoveAll(obsDir)
}

// StartOBS find process and then start him
func (mgr *OBSManager) StartOBS(name string) error {

	if mgr.processes == nil {
		log.WithFields(log.Fields{"type": consts.WrongModeError, "error": errWrongMode}).Error("starting OBS")
		return errWrongMode
	}

	proc := mgr.processes.Find(name)
	if proc == nil {
		err := fmt.Errorf(`OBS '%s' is not exists`, name)
		log.WithFields(log.Fields{"type": consts.OBSManagerError, "error": err}).Error("on find OBS process")
		return err
	}

	state := proc.GetState()
	if state == process.Stopped ||
		state == process.Exited ||
		state == process.Fatal {
		proc.Start(true)
		log.WithFields(log.Fields{"obs_name": name}).Info("OBS started")
		return nil
	}

	err := fmt.Errorf("OBS '%s' is %s", name, state)
	log.WithFields(log.Fields{"type": consts.OBSManagerError, "error": err}).Error("on starting OBS")
	return err
}

// StopOBS find process with definded name and then stop him
func (mgr *OBSManager) StopOBS(name string) error {

	if mgr.processes == nil {
		log.WithFields(log.Fields{"type": consts.WrongModeError, "error": errWrongMode}).Error("on stopping OBS process")
		return errWrongMode
	}

	proc := mgr.processes.Find(name)
	if proc == nil {
		err := fmt.Errorf(`OBS '%s' is not exists`, name)
		log.WithFields(log.Fields{"type": consts.OBSManagerError, "error": err}).Error("on find OBS process")
		return err
	}

	state := proc.GetState()
	if state == process.Running ||
		state == process.Starting {
		proc.Stop(true)
		log.WithFields(log.Fields{"obs_name": name}).Info("OBS is stoped")
		return nil
	}

	err := fmt.Errorf("OBS '%s' is %s", name, state)
	log.WithFields(log.Fields{"type": consts.OBSManagerError, "error": err}).Error("on stoping OBS")
	return err
}

func (mgr *OBSManager) createOBSDB(obsName, login, pass string) error {

	if err := model.DBConn.Exec(fmt.Sprintf(createRoleTemplate, login, pass)).Error; err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating OBS DB User")
		return err
	}

	if err := model.DBConn.Exec(fmt.Sprintf(createDBTemplate, obsName, login)).Error; err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating OBS DB")

		if err := model.GetDB(nil).Exec(fmt.Sprintf(dropDBRoleTemplate, login)).Error; err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err, "role": login}).Error("on deleting obs role")
			return err
		}
		return err
	}

	return nil
}

func (mgr *OBSManager) initOBSDir(obsName string) error {

	obsDirName := path.Join(mgr.childConfigsPath, obsName)
	if _, err := os.Stat(obsDirName); os.IsNotExist(err) {
		if err := os.Mkdir(obsDirName, 0700); err != nil {
			log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("creating OBS directory")
			return err
		}
	}

	return nil
}

func InitOBSManager() {
	if !conf.Config.IsOBSMaster() {
		return
	}

	execPath, err := os.Executable()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.OBSManagerError, "error": err}).Fatal("on determine executable path")
	}

	childConfigsPath, err := prepareWorkDir()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.OBSManagerError, "error": err}).Fatal("on prepare child configs folder")
	}

	Manager = &OBSManager{
		processes:        process.NewManager(),
		execPath:         execPath,
		childConfigsPath: childConfigsPath,
	}

	list, err := os.ReadDir(childConfigsPath)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "path": childConfigsPath}).Fatal("on read child OBS directory")
	}

	for _, item := range list {
		if item.IsDir() {
			procDir := path.Join(Manager.childConfigsPath, item.Name())
			commandStr := fmt.Sprintf(commandTemplate, Manager.execPath, filepath.Join(procDir, consts.DefaultConfigFile))
			log.Info(commandStr)
			confEntry := pConf.NewEntry(procDir)
			confEntry.Name = "program:" + item.Name()
			section := ini.NewSection(confEntry.Name)
			section.Add("command", commandStr)
			section.Add("redirect_stderr", "true")
			section.Add("autostart", "true")
			section.Add("autorestart", "true")

			proc := process.NewProcess("obsMaster", confEntry)
			Manager.processes.Add(item.Name(), proc)
			proc.Start(true)
		}
	}
}

func dropDb(name, role string) error {
	if err := model.DropDatabase(name); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err, "db_name": name}).Error("Deleting obs db")
		return err
	}

	if err := model.GetDB(nil).Exec(fmt.Sprintf(dropDBRoleTemplate, role)).Error; err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err, "role": role}).Error("on deleting obs role")
	}
	return nil
}

func dropOBSDir(configsPath, obsName string) error {
	path := path.Join(configsPath, obsName)
	if directoryExists(path) {
		os.RemoveAll(path)
	}

	log.WithFields(log.Fields{"path": path}).Error("droping dir is not exists")
	return nil
}

func directoryExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}

	return true
}

func checkOBSName(name string) error {

	name = strings.ToLower(name)

	for i, c := range name {
		if unicode.IsDigit(c) && i == 0 {
			return fmt.Errorf("the name cannot begin with a number")
		}
		if !unicode.IsDigit(c) && !unicode.Is(unicode.Latin, c) {
			return fmt.Errorf("Incorrect symbol")
		}
	}

	return nil
}

func (mgr *OBSManager) configByName(name string) (*conf.GlobalConfig, error) {
	path := path.Join(mgr.childConfigsPath)
	c := &conf.GlobalConfig{}
	err := conf.LoadConfigToVar(path, c)
	return c, err
}
