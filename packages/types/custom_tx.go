/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package types

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
)

// Transaction types.
const (
	FirstBlockTxType = iota + 1
	StopNetworkTxType
	SmartContractTxType
	DelayTxType
	UtxoTxType
	TransferSelfTxType
)

// ClientTxType is the frame a client sends a transaction in: the payload it signed and the
// signature. The node it sends to stores the transaction, with the time it takes it, in the form
// of its type; blocks and other nodes carry only that stored form.
const ClientTxType = 0x80

// FirstBlock is the header of first block transaction
type FirstBlock struct {
	KeyID                 int64
	Time                  int64
	PublicKey             []byte
	NodePublicKey         []byte
	StopNetworkCertBundle []byte
	Test                  int64
	PrivateBlockchain     uint64
	// HonorNodesOnly is the value of the platform parameter honor_nodes_only
	HonorNodesOnly uint64
	// AccountAlgorithms is the initial value of the platform parameter account_algorithms
	AccountAlgorithms string
	// BftConsensus is the value of the platform parameter bft_consensus
	BftConsensus uint64
	// HonorNodes is the initial value of the platform parameter honor_nodes: the validators of a
	// chain of the BFT consensus, which its first block names
	HonorNodes string
	// Supply is the amount of the coin of ecosystem 1 that may ever exist, in its smallest unit:
	// the allocations of the first block and the rest, which is not issued. Empty, it is
	// consts.TotalSupply.
	Supply string
	// Allocations are the amounts the first block credits to the accounts of ecosystem 1. None,
	// the founder takes consts.FounderAmount.
	Allocations []Allocation
	// ChainFormat is consts.ChainFormat of the binary that made the first block; a first block of
	// an earlier binary has none, 0
	ChainFormat uint64
}

// Allocation is an amount the first block credits to an account of ecosystem 1: a part of the
// supply, named, in its smallest unit. An allocation to the founder has no Conditions; any other
// credits an account the first block creates without a key, which only the contracts of its
// Conditions move (CallContractAs), as the conditions of 1_keys name them.
type Allocation struct {
	Name       string
	KeyID      int64
	Amount     string
	Conditions string
}

func (t *FirstBlock) TxType() byte { return FirstBlockTxType }

type StopNetwork struct {
	KeyID           int64
	Time            int64
	StopNetworkCert []byte
}

func (t *StopNetwork) TxType() byte { return StopNetworkTxType }

// Header is contain header data
type Header struct {
	ID          int
	EcosystemID int64
	KeyID       int64
	Time        int64
	NetworkID   int64
	PublicKey   []byte
}

type TransferSelf struct {
	Value  string
	Source string
	Target string
}

// UTXO Transfer
type UTXO struct {
	ToID    int64
	Value   string
	Comment string
}

// SmartTransaction is storing smart contract data
type SmartTransaction struct {
	*Header
	MaxSum       string
	PayOver      string
	Lang         string
	Expedite     string
	SignedBy     int64
	TransferSelf *TransferSelf
	UTXO         *UTXO
	Params       map[string]any
}

func (s *SmartTransaction) TxType() byte {
	if s.TransferSelf != nil {
		return TransferSelfTxType
	}
	if s.UTXO != nil {
		return UtxoTxType
	}
	return SmartContractTxType
}

func (s *SmartTransaction) WithPrivate(privateKey []byte, internal bool) error {
	var (
		publicKey []byte
		err       error
	)
	if publicKey, err = crypto.NodePrivateToPublic(privateKey); err != nil {
		log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("converting node private key to public")
		return err
	}
	s.PublicKey = crypto.NodeAccountKey(publicKey).Bytes()
	if internal {
		s.SignedBy = crypto.Address(publicKey)
	}
	if s.NetworkID != conf.Config.LocalConf.NetworkID {
		return fmt.Errorf("error networkid invalid")
	}
	return nil
}

func (s *SmartTransaction) Unmarshal(buffer []byte) error {
	return msgpack.Unmarshal(buffer, s)
}

func (s *SmartTransaction) Marshal() ([]byte, error) {
	return msgpack.Marshal(s)
}

func (t SmartTransaction) Hash() ([]byte, error) {
	b, err := t.Marshal()
	if err != nil {
		return nil, err
	}
	return crypto.DoubleHash(b), nil
}

func (txSmart *SmartTransaction) Validate() error {
	if len(txSmart.Expedite) > 0 {
		expedite, err := decimal.NewFromString(txSmart.Expedite)
		if err != nil {
			return fmt.Errorf("wrong expedite format")
		}
		if expedite.LessThan(decimal.Zero) {
			return fmt.Errorf("expedite fee must be greater than 0")
		}
	}
	if len(strings.TrimSpace(txSmart.Lang)) > 2 {
		return fmt.Errorf(`localization size is greater than 2`)
	}
	if len(txSmart.MaxSum) > 0 && converter.StrToInt64(txSmart.MaxSum) <= 0 {
		return fmt.Errorf(`maxsum must be greater than 0`)
	}
	if txSmart.NetworkID != conf.Config.LocalConf.NetworkID {
		return fmt.Errorf("error networkid invalid")
	}

	if txSmart.TransferSelf != nil {
		if ok, _ := regexp.MatchString("^\\d+$", txSmart.TransferSelf.Value); !ok {
			return errors.New("error TransferSelf Value must be a positive integer")
		}
		if value, err := decimal.NewFromString(txSmart.TransferSelf.Value); err != nil || value.LessThanOrEqual(decimal.Zero) {
			return errors.New("error TransferSelf Value must be greater than zero")
		}
		return nil
	}
	if txSmart.UTXO != nil {
		if converter.IDToAddress(txSmart.UTXO.ToID) == `invalid` {
			return errors.New("error UTXO ToID must be a valid address")
		}
		if ok, _ := regexp.MatchString("^\\d+$", txSmart.UTXO.Value); !ok {
			return errors.New("error UTXO Value must be a positive integer")
		}
		if value, err := decimal.NewFromString(txSmart.UTXO.Value); err != nil || value.LessThanOrEqual(decimal.Zero) {
			return errors.New("error UTXO Value must be greater than zero")
		}
		return nil
	}
	return nil
}
