/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"errors"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/common/crypto/x509"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

var (
	messageNetworkStopping = "Attention! The network is stopped!"

	ErrNetworkStopping = errors.New("network is stopping")
)

type StopNetworkTransaction struct {
	Logger  *log.Entry
	Data    types.StopNetwork
	Cert    *x509.Cert
	TxHash  []byte
	Payload []byte // transaction binary data
}

func (s *StopNetworkTransaction) txType() byte                { return types.StopNetworkTxType }
func (s *StopNetworkTransaction) txHash() []byte              { return s.TxHash }
func (s *StopNetworkTransaction) txPayload() []byte           { return s.Payload }
func (s *StopNetworkTransaction) txTime() int64               { return int64(s.Data.Time) }
func (s *StopNetworkTransaction) txKeyID() int64              { return s.Data.KeyID }
func (s *StopNetworkTransaction) txExpedite() decimal.Decimal { return decimal.Decimal{} }

func (s *StopNetworkTransaction) Init(*Transaction) error {
	return nil
}

func (s *StopNetworkTransaction) Validate() error {
	if err := s.validate(); err != nil {
		s.Logger.WithError(err).Error("validating tx")
		return err
	}

	return nil
}

func (s *StopNetworkTransaction) validate() error {
	data := s.Data
	cert, err := x509.ParseCert(data.StopNetworkCert)
	if err != nil {
		return err
	}

	fbdata, err := syspar.GetFirstBlockData()
	if err != nil {
		return err
	}

	if err = cert.Validate(fbdata.StopNetworkCertBundle); err != nil {
		return err
	}

	s.Cert = cert
	return nil
}

func (s *StopNetworkTransaction) Action(t *Transaction) error {
	// Allow execute transaction, if the certificate was used
	if s.Cert.EqualBytes(consts.UsedStopNetworkCerts...) {
		return nil
	}

	// Set the node in a pause state
	//node.PauseNodeActivity(node.PauseTypeStopingNetwork)

	s.Logger.Warn(messageNetworkStopping)
	return ErrNetworkStopping
}

func (s *StopNetworkTransaction) TxRollback() error {
	return nil
}

func (s *StopNetworkTransaction) Unmarshal(buffer *bytes.Buffer) error {
	buffer.UnreadByte()
	s.Payload = buffer.Bytes()
	s.TxHash = crypto.DoubleHash(s.Payload)
	if err := converter.BinUnmarshal(&s.Payload, &s.Data); err != nil {
		return err
	}
	return nil
}
