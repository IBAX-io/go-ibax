package conf

type (
	// HostPort endpoint in form "str:int"
	HostPort struct {
		Host string // ipaddr, hostname, or "0.0.0.0"
		Port int    // must be in range 1..65535
	}

	// DBConfig database connection parameters
	DBConfig struct {
		Name            string
		Host            string
		Port            int
		User            string
		Password        string
		LockTimeout     int // lock_timeout in milliseconds
		IdleInTxTimeout int // postgres parameter idle_in_transaction_session_timeout
		MaxIdleConns    int // sets the maximum number of connections in the idle connection pool
		MaxOpenConns    int // sets the maximum number of open connections to the database
	}

	//RedisConfig get redis information from config.yml
	RedisConfig struct {
		Enable   bool
		Host     string
		Port     int
		Password string
		DbName   int
	}

	// MetricsConfig is where the node serves its Prometheus metrics and the Go profiler
	MetricsConfig struct {
		// Listen is a loopback host and a port; empty serves nothing
		Listen string
	}

	// TxPoolConfig bounds the transactions a node keeps in memory for the blocks to come. Zero
	// takes the defaults of the transaction package.
	TxPoolConfig struct {
		MaxCount int   // transactions of clients and other nodes
		MaxBytes int64 // bytes of their data
		// MaxPerKey is the transactions of clients and other nodes an account has in the pool
		MaxPerKey int
		// ReserveCount and ReserveBytes bound, beyond MaxCount and MaxBytes, the transactions the
		// node makes and the evidences validators sign: a pool full of transactions of clients
		// still takes them
		ReserveCount int
		ReserveBytes int64
		// BadMinutes is how long the node answers the status of a transaction it refused
		BadMinutes int
	}

	// StorageConfig is what the node keeps of the rows out of the window of the chain
	StorageConfig struct {
		// Archive keeps them, in the archive schema, for the API and the synchronization
		Archive bool
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
		Host     string
		Port     int
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
		// TimeSource is the NTP server, host:port, a validator of a chain of the BFT consensus
		// checks its clock against
		TimeSource string
	}
	// GlobalConfig is storing all startup config as global struct
	GlobalConfig struct {
		KeyID        int64  `toml:"-"`
		ConfigPath   string `toml:"-"`
		TestRollBack bool   `toml:"-"`
		FuncBench    bool   `toml:"-"`
		LocalConf    LocalConfig
		DirPathConf  DirectoryConfig
		BootNodes    BootstrapNodeConfig
		TLSConf      TLSConfig
		TCPServer    HostPort
		HTTP         HostPort
		JsonRPC      struct {
			Enabled   bool
			Namespace string
		}
		DB             DBConfig
		Redis          RedisConfig
		Metrics        MetricsConfig
		TxPool         TxPoolConfig
		Storage        StorageConfig
		Centrifugo     CentrifugoConfig
		Log            LogConfig
		TokenMovement  TokenMovementConfig
		BanKey         BanKeyConfig
		CryptoSettings CryptoSettings
	}
)
