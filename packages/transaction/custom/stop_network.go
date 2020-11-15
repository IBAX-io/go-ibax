/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package custom

import (
	"errors"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/service"
	"github.com/IBAX-io/go-ibax/packages/utils"
	"github.com/IBAX-io/go-ibax/packages/utils/tx"
)

type StopNetworkTransaction struct {
	Logger *log.Entry
	Data   interface{}
	Cert   *utils.Cert
}

func (t *StopNetworkTransaction) Init() error {
	return nil
}

func (t *StopNetworkTransaction) Validate() error {
	if err := t.validate(); err != nil {
		t.Logger.WithError(err).Error("validating tx")
		return err
	}

	return nil
}

func (t *StopNetworkTransaction) validate() error {
	data := t.Data.(*consts.StopNetwork)
	cert, err := utils.ParseCert(data.StopNetworkCert)
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

	t.Cert = cert
	return nil
}

func (t *StopNetworkTransaction) Action() error {
	// Allow execute transaction, if the certificate was used
	if t.Cert.EqualBytes(consts.UsedStopNetworkCerts...) {
		return nil
	}

	// Set the node in a pause state
	service.PauseNodeActivity(service.PauseTypeStopingNetwork)

	t.Logger.Warn(messageNetworkStopping)
	return ErrNetworkStopping
}

func (t *StopNetworkTransaction) Rollback() error {
	return nil
}

func (t StopNetworkTransaction) Header() *tx.Header {
	return nil
}
