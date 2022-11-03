/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/BurntSushi/toml"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Config global parameters
var Config GlobalConfig

// Str converts HostPort pair to string format
func (h HostPort) Str() string {
	return fmt.Sprintf("%s:%d", h.Host, h.Port)
}

// GetPidPath returns path to pid file
func (c *GlobalConfig) GetPidPath() string {
	return c.DirPathConf.PidFilePath
}

// LoadConfig from configFile
// the function has side effect updating global var Config
func LoadConfig(path string) error {
	err := LoadConfigToVar(path, &Config)
	if err != nil {
		log.WithError(err).Fatal("Loading config")
	}
	log.WithFields(log.Fields{"path": path}).Info("Loading config")
	registerCrypto(Config.CryptoSettings)
	return nil
}

func LoadConfigToVar(path string, v *GlobalConfig) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return errors.Errorf("Unable to load config file %s", path)
	}

	viper.SetConfigFile(path)
	err = viper.ReadInConfig()
	if err != nil {
		return errors.Wrapf(err, "reading config")
	}

	err = viper.Unmarshal(v)
	if err != nil {
		return errors.Wrapf(err, "marshalling config to global struct variable")
	}
	return nil
}

// GetConfigFromPath read config from path and returns GlobalConfig struct
func GetConfigFromPath(path string) (*GlobalConfig, error) {
	log.WithFields(log.Fields{"path": path}).Info("Loading clb config")

	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, errors.Errorf("Unable to load config file %s", path)
	}

	viper.SetConfigFile(path)
	err = viper.ReadInConfig()
	if err != nil {
		return nil, errors.Wrapf(err, "reading config")
	}

	c := &GlobalConfig{}
	err = viper.Unmarshal(c)
	if err != nil {
		return c, errors.Wrapf(err, "marshalling config to global struct variable")
	}

	return c, nil
}

// SaveConfig save global parameters to configFile
func SaveConfig(path string) error {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.Mkdir(dir, 0775)
		if err != nil {
			return errors.Wrapf(err, "creating dir %s", dir)
		}
	}

	cf, err := os.Create(path)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("Create config file failed")
		return err
	}
	defer cf.Close()

	err = toml.NewEncoder(cf).Encode(Config)
	if err != nil {
		return err
	}
	return nil
}

// FillRuntimePaths fills paths from runtime parameters
func FillRuntimePaths() error {
	if Config.DirPathConf.DataDir == "" {
		//cwd, err := os.Getwd()
		//if err != nil {
		//	return errors.Wrapf(err, "getting current wd")
		//}

		//Config.DataDir = filepath.Join(cwd, consts.DefaultWorkdirName)
		Config.DirPathConf.DataDir = filepath.Join(consts.DefaultWorkdirName)
	}

	if Config.DirPathConf.KeysDir == "" {
		Config.DirPathConf.KeysDir = Config.DirPathConf.DataDir
	}

	if Config.DirPathConf.TempDir == "" {
		Config.DirPathConf.TempDir = filepath.Join(os.TempDir(), consts.DefaultTempDirName)
	}

	if Config.DirPathConf.FirstBlockPath == "" {
		Config.DirPathConf.FirstBlockPath = filepath.Join(Config.DirPathConf.DataDir, consts.FirstBlockFilename)
	}

	if Config.DirPathConf.PidFilePath == "" {
		Config.DirPathConf.PidFilePath = filepath.Join(Config.DirPathConf.DataDir, consts.DefaultPidFilename)
	}

	if Config.DirPathConf.LockFilePath == "" {
		Config.DirPathConf.LockFilePath = filepath.Join(Config.DirPathConf.DataDir, consts.DefaultLockFilename)
	}

	return nil
}

// FillRuntimeKey fills parameters of keys from runtime parameters
func FillRuntimeKey() error {
	keyIDFileName := filepath.Join(Config.DirPathConf.KeysDir, consts.KeyIDFilename)
	keyIDBytes, err := os.ReadFile(keyIDFileName)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.IOError, "error": err, "path": keyIDFileName}).Error("reading KeyID file")
		return err
	}

	Config.KeyID, err = strconv.ParseInt(string(keyIDBytes), 10, 64)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "value": string(keyIDBytes)}).Error("converting keyID to int")
		return errors.New("converting keyID to int")
	}

	return nil
}

// GetNodesAddr returns address of nodes
func GetNodesAddr() []string {
	return Config.BootNodes.NodesAddr[:]
}

func registerCrypto(c CryptoSettings) {
	crypto.InitAsymAlgo(c.Cryptoer)
	crypto.InitHashAlgo(c.Hasher)
}
