/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package types

// Transaction types.
const (
	FirstBlockTxType = iota + 1
	StopNetworkTxType
	SmartContractTxType
)

// FirstBlock is the header of first block transaction
type FirstBlock struct {
	KeyID                 int64
	PublicKey             []byte
	NodePublicKey         []byte
	StopNetworkCertBundle []byte
	Test                  int64
	PrivateBlockchain     uint64
}

func (t *FirstBlock) TxType() byte { return FirstBlockTxType }

type StopNetwork struct {
	KeyID           int64
	StopNetworkCert []byte
}

func (t *StopNetwork) TxType() byte { return StopNetworkTxType }

// Header is contain header data
type Header struct {
	ID          int
	EcosystemID int64
	KeyID       int64
	NetworkID   int64
	PublicKey   []byte
}

// SmartTransaction is storing smart contract data
type SmartTransaction struct {
	*Header
	TokenEcosystems map[int64]interface{}
	MaxSum          string
	PayOver         string
	Lang            string
	Expedite        string
	SignedBy        int64
	Params          map[string]interface{}
}

func (s *SmartTransaction) TxType() byte { return SmartContractTxType }
