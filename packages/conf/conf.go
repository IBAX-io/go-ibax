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

type (
	// HostPort endpoint in form "str:int"
	HostPort struct {
		Host string // ipaddr, hostname, or "0.0.0.0"
		Port int    // must be in range 1..65535
	}

	// DBConfig database connection parameters
	DBConfig struct {
		Name string
		HostPort
		User            string
		Password        string
		LockTimeout     int // lock_timeout in milliseconds
		IdleInTxTimeout int // postgres parameter idle_in_transaction_session_timeout
		MaxIdleConns    int // sets the maximum number of connections in the idle connection pool
		MaxOpenConns    int // sets the maximum number of open connections to the database
	}

	//RedisConfig get redis information from config.yml
	RedisConfig struct {
		Enable bool
		HostPort
		Password string
		DbName   int
	}

	// StatsDConfig statd connection parameters
	StatsDConfig struct {
		HostPort
		Name string
	}

	// CentrifugoConfig connection params
	CentrifugoConfig struct {
		Secret string
		URL    string
		Key    string
	}

	// Syslog represents parameters of syslog
	Syslog struct {
		Facility string
		Tag      string
	}

	// LogConfig represents parameters of log
	LogConfig struct {
		LogTo     string
		LogLevel  string
		LogFormat string
		Syslog    Syslog
	}

	// TokenMovementConfig smtp config for token movement
	TokenMovementConfig struct {
		HostPort
		Username string
		Password string
		To       string
		From     string
		Subject  string
	}

	// BanKeyConfig parameters
	BanKeyConfig struct {
		BadTime int // control time period in minutes
		BanTime int // ban time in minutes
		BadTx   int // maximum bad tx during badTime minutes
	}

	IpfsConfig struct {
		Enabled bool
		Host    string
	}

	TLSConfig struct {
		Enabled bool   // TLS is on/off. It is required for https
		TLSCert string // TLSCert is a filepath of the fullchain of certificate.
		TLSKey  string // TLSKey is a filepath of the private key.
	}

	DirectoryConfig struct {
		DataDir        string // application work dir (cwd by default)
		PidFilePath    string
		LockFilePath   string
		TempDir        string // temporary dir
		KeysDir        string // place for private keys files: NodePrivateKey, PrivateKey
		FirstBlockPath string
	}

	BootstrapNodeConfig struct {
		NodesAddr []string
	}

	CryptoSettings struct {
		Cryptoer string
		Hasher   string
	}
	//LocalConfig TODO: uncategorized
	LocalConfig struct {
		RunNodeMode           string
		HTTPServerMaxBodySize int64
		NetworkID             int64
		MaxPageGenerationTime int64 // in milliseconds
	}
)

// Str converts HostPort pair to string format
func (h HostPort) Str() string {
	return fmt.Sprintf("%s:%d", h.Host, h.Port)
}

// GlobalConfig is storing all startup config as global struct
type GlobalConfig struct {
	KeyID          int64  `toml:"-"`
	ConfigPath     string `toml:"-"`
	TestRollBack   bool   `toml:"-"`
	FuncBench      bool   `toml:"-"`
	LocalConf      LocalConfig
	DirPathConf    DirectoryConfig
	BootNodes      BootstrapNodeConfig
	TLSConf        TLSConfig
	TCPServer      HostPort
	HTTP           HostPort
	DB             DBConfig
	Redis          RedisConfig
	StatsD         StatsDConfig
	Centrifugo     CentrifugoConfig
	Log            LogConfig
	TokenMovement  TokenMovementConfig
	BanKey         BanKeyConfig
	IpfsConf       IpfsConfig
	CryptoSettings CryptoSettings
}

// Config global parameters
var Config GlobalConfig

// GetPidPath returns path to pid file
func (c *GlobalConfig) GetPidPath() string {
	return c.DirPathConf.PidFilePath
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

func IpfsEnabled() bool {
	return Config.IpfsConf.Enabled
}

func IpfsHost() string {
	return Config.IpfsConf.Host
}

// GetNodesAddr returns addreses of nodes
func GetNodesAddr() []string {
	return Config.BootNodes.NodesAddr[:]
}

func registerCrypto(c CryptoSettings) {
	crypto.InitCurve(c.Cryptoer)
	crypto.InitHash(c.Hasher)
}
