/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"errors"
	"time"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/vmihailenco/msgpack/v5"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/common/crypto/x509"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

var (
	messageNetworkStopping = "Attention! The network is stopped!"

	ErrNetworkStopping = errors.New("network is stopping")
)

type StopNetworkParser struct {
	Logger    *log.Entry
	Data      *types.StopNetwork
	Cert      *x509.Cert
	Timestamp int64
	TxHash    []byte
	Payload   []byte // transaction binary data
}

func (s *StopNetworkParser) txType() byte                { return s.Data.TxType() }
func (s *StopNetworkParser) txHash() []byte              { return s.TxHash }
func (s *StopNetworkParser) txPayload() []byte           { return s.Payload }
func (s *StopNetworkParser) txTime() int64               { return s.Timestamp }
func (s *StopNetworkParser) txKeyID() int64              { return s.Data.KeyID }
func (s *StopNetworkParser) txExpedite() decimal.Decimal { return decimal.Decimal{} }
func (s *StopNetworkParser) setTimestamp()               { s.Timestamp = time.Now().UnixMilli() }

func (s *StopNetworkParser) Init(in *InToCxt) error {
	return nil
}

func (s *StopNetworkParser) Validate() error {
	if err := s.validate(); err != nil {
		s.Logger.WithError(err).Error("validating tx")
		return err
	}

	return nil
}

func (s *StopNetworkParser) validate() error {
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

func (s *StopNetworkParser) Action(in *InToCxt, out *OutCtx) (err error) {
	// Allow execute transaction, if the certificate was used
	if s.Cert.EqualBytes(consts.UsedStopNetworkCerts...) {
		return nil
	}

	// Set the node in a pause state
	//node.PauseNodeActivity(node.PauseTypeStopingNetwork)

	s.Logger.Warn(messageNetworkStopping)
	return ErrNetworkStopping
}

func (s *StopNetworkParser) TxRollback() error                                      { return nil }
func (s *StopNetworkParser) SysUpdateWorker(dbTx *sqldb.DbTransaction) error        { return nil }
func (s *StopNetworkParser) SysTableColByteaWorker(dbTx *sqldb.DbTransaction) error { return nil }
func (s *StopNetworkParser) FlushVM()                                               {}

func (s *StopNetworkParser) BinMarshal(data *types.StopNetwork) ([]byte, error) {
	s.setTimestamp()
	s.Data = data
	var (
		buf []byte
		err error
	)
	buf, err = msgpack.Marshal(data)
	if err != nil {
		return nil, err
	}
	s.Payload = buf
	s.TxHash = crypto.DoubleHash(s.Payload)

	err = s.validate()
	if err != nil {
		return nil, err
	}
	buf, err = msgpack.Marshal(s)
	if err != nil {
		return nil, err
	}
	buf = append([]byte{s.txType()}, buf...)
	return buf, nil
}

func (s *StopNetworkParser) Unmarshal(buffer *bytes.Buffer) error {
	buffer.UnreadByte()
	if err := msgpack.Unmarshal(buffer.Bytes()[1:], s); err != nil {
		return err
	}
	return nil
}
