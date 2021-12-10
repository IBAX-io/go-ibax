/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package consts

import (
	"fmt"
	"strings"
	"time"
)

// VERSION is current version
const VERSION = "1.3.0"

const BvRollbackHash = 2
const BvIncludeRollbackHash = 3

// BlockVersion is block version
const BlockVersion = BvIncludeRollbackHash

// DEFAULT_TCP_PORT used when port number missed in host addr
const DEFAULT_TCP_PORT = 7078

// FounderAmount is the starting amount of founder
//const FounderAmount = 5250000
const FounderAmount = 5250000

// MoneyDigits is numbers of digits for tokens 1000000000000
const MoneyDigits = 12

// WAIT_CONFIRMED_NODES is used in confirmations
const WAIT_CONFIRMED_NODES = 10

// MIN_CONFIRMED_NODES The number of nodes which should have the same block as we have for regarding this block belongs to the major part of DC-net. For get_confirmed_block_id()
const MIN_CONFIRMED_NODES = 0

// DOWNLOAD_CHAIN_TRY_COUNT is number of attempt
const DOWNLOAD_CHAIN_TRY_COUNT = 10

// MAX_TX_FORW How fast could the time of transaction pass
const MAX_TX_FORW = 600

// MAX_TX_BACK transaction may wander in the net for a day and then get into a block
const MAX_TX_BACK = 86400

// ERROR_TIME is error time
const ERROR_TIME = 1

// ROUND_FIX is rounding constant
const ROUND_FIX = 0.00000000001

// READ_TIMEOUT is timeout for TCP
const READ_TIMEOUT = 20

// WRITE_TIMEOUT is timeout for TCP
const WRITE_TIMEOUT = 20

// DATA_TYPE_MAX_BLOCK_ID is block id max datatype
const DATA_TYPE_MAX_BLOCK_ID = 10

// DATA_TYPE_BLOCK_BODY is body block datatype
const DATA_TYPE_BLOCK_BODY = 7

// AddressLength is length of address
const AddressLength = 20

// PubkeySizeLength is pubkey length
const PubkeySizeLength = 64

// PrivkeyLength is privkey length
const PrivkeyLength = 32

// BlockSize is size of block
const BlockSize = 16

// HashSize is size of hash
const HashSize = 32

const AvailableBCGap = 4

const DefaultNodesConnectDelay = 6

const MaxTXAttempt = 10

// ChainSize 1M = 1048576 byte
const ChainSize = 1 << 20

// ApiPath is the beginning of the api url
var ApiPath = `/api/v2/`

// BuildInfo should be defined through -ldflags
var BuildInfo string

const (
	// DefaultConfigFile name of config file (toml format)
	DefaultConfigFile = "config.toml"

	// DefaultWorkdirName name of working directory
	DefaultWorkdirName = "data"

	// DefaultPidFilename is default filename of pid file
	DefaultPidFilename = "go-ibax.pid"

	// DefaultLockFilename is default filename of lock file
	DefaultLockFilename = "go-ibax.lock"

	// FirstBlockFilename name of first block binary file
	FirstBlockFilename = "1block"

	// PrivateKeyFilename name of wallet private key file
	PrivateKeyFilename = "PrivateKey"

	// PublicKeyFilename name of wallet public key file
	PublicKeyFilename = "PublicKey"

	// NodePrivateKeyFilename name of node private key file
	NodePrivateKeyFilename = "NodePrivateKey"

	// NodePublicKeyFilename name of node public key file
	NodePublicKeyFilename = "NodePublicKey"

	// KeyIDFilename generated KeyID
	KeyIDFilename = "KeyID"

	// RollbackResultFilename rollback result file
	RollbackResultFilename = "rollback_result"

	// FromToPerDayLimit day limit token transfer between accounts
	FromToPerDayLimit = 10000

	// TokenMovementQtyPerBlockLimit block limit token transfer
	TokenMovementQtyPerBlockLimit = 100

	// TCPConnTimeout timeout of tcp connection
	TCPConnTimeout = 5 * time.Second

	// TxRequestExpire is expiration time for request of transaction
	TxRequestExpire = 1 * time.Minute

	// DefaultTempDirName is default name of temporary directory
	DefaultTempDirName = "ibax-temp"

	// DefaultOBS allways is 1
	DefaultOBS = 1

	// MoneyLength is the maximum number of digits in money value
	MoneyLength = 30

	DefaultTokenEcosystem = 1

	HTTPServerMaxBodySize = 1 << 20

	// ShiftContractID is the offset of tx identifiers
	ShiftContractID = 5000

	// ContractList is the number of contracts per page on loading
	ContractList = 200

	// Guest key
	GuestPublic  = "489347a1205c818d9a02f285faaedd0122a56138e3d985f5e1b4f6a9470f90f692a00a3453771dd7feea388ceb7aefeaf183e299c70ad1aecb7f870bfada3b86"
	GuestKey     = "4544233900443112470"
	GuestAddress = "0454-4233-9004-4311-2470"

	// StatusMainPage is a status for Main Page
	StatusMainPage = `2`

	NoneOBS     = "none"
	DBFindLimit = 10000
)

const (
	SavePointMarkBlock = "block"
	SavePointMarkTx    = "tx"
)

func Version() string {
	return strings.TrimSpace(strings.Join([]string{VERSION, BuildInfo}, " "))
}

func SetSavePointMarkBlock(idTx int) string {
	return fmt.Sprintf("\"%s-%d\";", SavePointMarkBlock, idTx)
}
