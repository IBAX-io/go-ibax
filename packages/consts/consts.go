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

// DefaultTcpPort used when port number missed in host addr
const DefaultTcpPort = 7078

// FounderAmount is the starting amount of founder
const FounderAmount = 5250000

// MoneyDigits is numbers of digits for tokens 1000000000000
const MoneyDigits = 12

// WaitConfirmedNodes is used in confirmations
const WaitConfirmedNodes = 10

// MinConfirmedNodes The number of nodes which should have the same block as we have for regarding this block belongs to the major part of DC-net. For get_confirmed_block_id()
const MinConfirmedNodes = 0

// MaxTxForw How fast could the time of transaction pass
const MaxTxForw = 600

// MaxTxBack transaction may wander in the net for a day and then get into a block
const MaxTxBack = 86400

// RoundFix is rounding constant
const RoundFix = 0.00000000001

// ReadTimeout is timeout for TCP
const ReadTimeout = 20

// WriteTimeout is timeout for TCP
const WriteTimeout = 20

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

// DefaultTokenSymbol define default token symbol
const DefaultTokenSymbol = "IBXC"

// DefaultTokenName define default token name
const DefaultTokenName = "IBAX Coin"

// DefaultEcosystemName define default ecosystem name
const DefaultEcosystemName = "platform ecosystem"

// ApiPath is the beginning of the api url
var ApiPath = `/api/v2/`

// BuildInfo should be defined through -ldflags
var BuildInfo string

const (
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

	// DefaultCLB always is 1
	DefaultCLB = 1

	// MoneyLength is the maximum number of digits in money value
	MoneyLength = 30

	DefaultTokenEcosystem = 1

	// ShiftContractID is the offset of tx identifiers
	ShiftContractID = 5000

	// ContractList is the number of contracts per page on loading
	ContractList = 200

	// Guest key
	GuestPublic  = "ef0ab117793962b7b3ee8d2ae94b58bbd7db1aa856a7dc623fdb28ad530090b0bcf5cb81b4d6912a249f1ab30921f414ad88383208cd8ba26ae2a9c3eb543772"
	GuestKey     = "-110277540701013350"
	GuestAddress = "1833-6466-5330-0853-8266"

	// StatusMainPage is a status for Main Page
	StatusMainPage = `2`

	NoneCLB     = "none"
	DBFindLimit = 10000

	HonorNodeMode     = 1
	CandidateNodeMode = 2
)

const (
	SavePointMarkBlock = "block"
	SavePointMarkTx    = "tx"
)

func Version() string {
	return strings.TrimSpace(strings.Join([]string{VERSION, BuildInfo}, " "))
}

func SetSavePointMarkBlock(idTx string) string {
	return fmt.Sprintf("\"%s-%s\";", SavePointMarkBlock, idTx)
}

const (
	UTXO_Type_First_Block  = 1 //Initialize the first block
	UTXO_Type_Self_UTXO    = 11
	UTXO_Type_Self_Account = 12
	UTXO_Type_Packaging    = 20
	UTXO_Type_Taxes        = 21
	UTXO_Type_Output       = 22
	UTXO_Type_Combustion   = 23
	UTXO_Type_Transfer     = 26
)
