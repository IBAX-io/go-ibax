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
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/crypto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// HostPort endpoint in form "str:int"
type HostPort struct {
	Host string // ipaddr, hostname, or "0.0.0.0"
	Port int    // must be in range 1..65535
}

// Str converts HostPort pair to string format
func (h HostPort) Str() string {
	return fmt.Sprintf("%s:%d", h.Host, h.Port)
}

// DBConfig database connection parameters
type DBConfig struct {
	Name            string
	Host            string // ipaddr, hostname, or "0.0.0.0"
	Port            int    // must be in range 1..65535
	User            string
	Password        string
	LockTimeout     int // lock_timeout in milliseconds
	IdleInTxTimeout int // postgres parameter idle_in_transaction_session_timeout
	MaxIdleConns    int // sets the maximum number of connections in the idle connection pool
	MaxOpenConns    int // sets the maximum number of open connections to the database
}

//RedisConfig get redis information from config.yml
type RedisConfig struct {
	Enable   bool
	Host     string
	Port     string
	Password string
	DbName   int
}

// StatsDConfig statd connection parameters
type StatsDConfig struct {
	Host string // ipaddr, hostname, or "0.0.0.0"
	Port int    // must be in range 1..65535
	Name string
}

// CentrifugoConfig connection params
type CentrifugoConfig struct {
	Secret string
	URL    string
	Key    string
}

// Syslog represents parameters of syslog
type Syslog struct {
	Facility string
	Tag      string
}

// Log represents parameters of log
type LogConfig struct {
	LogTo     string
	LogLevel  string
	LogFormat string
	Syslog    Syslog
}

// TokenMovementConfig smtp config for token movement
type TokenMovementConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	To       string
	From     string
	Subject  string
}

// BanKey parameters
type BanKeyConfig struct {
	BadTime int // control time period in minutes
	BanTime int // ban time in minutes
	BadTx   int // maximum bad tx during badTime minutes
}
type GFilesConfig struct {
	GFiles bool //GFiles is on/off. It is required for GFiles
	Host   string
}

// GlobalConfig is storing all startup config as global struct
type GlobalConfig struct {
	KeyID        int64  `toml:"-"`
	ConfigPath   string `toml:"-"`
	TestRollBack bool   `toml:"-"`
	FuncBench    bool   `toml:"-"`

	PidFilePath           string
	LockFilePath          string
	DataDir               string // application work dir (cwd by default)
	KeysDir               string // place for private keys files: NodePrivateKey, PrivateKey
	TempDir               string // temporary dir
	FirstBlockPath        string
	TLS                   bool   // TLS is on/off. It is required for https
	TLSCert               string // TLSCert is a filepath of the fullchain of certificate.
	TLSKey                string // TLSKey is a filepath of the private key.
	OBSMode               string
	HTTPServerMaxBodySize int64
	NetworkID             int64

	MaxPageGenerationTime int64 // in milliseconds

	TCPServer HostPort
	HTTP      HostPort

	DB             DBConfig
	Redis          RedisConfig
	StatsD         StatsDConfig
	Centrifugo     CentrifugoConfig
	Log            LogConfig
	TokenMovement  TokenMovementConfig
	BanKey         BanKeyConfig
	GFiles         GFilesConfig
	NodesAddr      []string
	CryptoSettings CryptoSettings
}

type CryptoSettings struct {
	Cryptoer string
	Hasher   string
}

// Config global parameters
var Config GlobalConfig

// GetPidPath returns path to pid file
func (c *GlobalConfig) GetPidPath() string {
	return c.PidFilePath
}

// LoadConfig from configFile
// the function has side effect updating global var Config
func LoadConfig(path string) error {
	log.WithFields(log.Fields{"path": path}).Info("Loading config")
	err := LoadConfigToVar(path, &Config)
	if err != nil {
		panic(err)
	}
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
	log.WithFields(log.Fields{"path": path}).Info("Loading obs config")

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
	if Config.DataDir == "" {
		//cwd, err := os.Getwd()
		//if err != nil {
		//	return errors.Wrapf(err, "getting current wd")
		//}

		//Config.DataDir = filepath.Join(cwd, consts.DefaultWorkdirName)
		Config.DataDir = filepath.Join(consts.DefaultWorkdirName)
	}

	if Config.KeysDir == "" {
		Config.KeysDir = Config.DataDir
	}

	if Config.TempDir == "" {
		Config.TempDir = filepath.Join(os.TempDir(), consts.DefaultTempDirName)
	}

	if Config.FirstBlockPath == "" {
		Config.FirstBlockPath = filepath.Join(Config.DataDir, consts.FirstBlockFilename)
	}

	if Config.PidFilePath == "" {
		Config.PidFilePath = filepath.Join(Config.DataDir, consts.DefaultPidFilename)
	}

	if Config.LockFilePath == "" {
		Config.LockFilePath = filepath.Join(Config.DataDir, consts.DefaultLockFilename)
	}

	return nil
}

// FillRuntimeKey fills parameters of keys from runtime parameters
func FillRuntimeKey() error {
	keyIDFileName := filepath.Join(Config.KeysDir, consts.KeyIDFilename)
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

// GetGFiles returns bool of gfiles
func GetGFiles() bool {
	return Config.GFiles.GFiles
}

// GetGFilesHost returns host of gfiles
func GetGFilesHost() string {
	return Config.GFiles.Host
}

// GetNodesAddr returns addreses of nodes
func GetNodesAddr() []string {
	return Config.NodesAddr[:]
}

// IsOBS check running mode
func (c GlobalConfig) IsOBS() bool {
	return RunMode(c.OBSMode).IsOBS()
}

// IsOBSMaster check running mode
func (c GlobalConfig) IsOBSMaster() bool {
	return RunMode(c.OBSMode).IsOBSMaster()
}

// IsSupportingOBS check running mode
func (c GlobalConfig) IsSupportingOBS() bool {
	return RunMode(c.OBSMode).IsSupportingOBS()
}

// IsNode check running mode
func (c GlobalConfig) IsNode() bool {
	return RunMode(c.OBSMode).IsNode()
}

//
//Add sub node processing
// IsSubNode check running mode
func (c GlobalConfig) IsSubNode() bool {
	return RunMode(c.OBSMode).IsSubNode()
}

func registerCrypto(c CryptoSettings) {
	crypto.InitCurve(c.Cryptoer)
	crypto.InitHash(c.Hasher)
}
