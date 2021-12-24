/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package x509

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
)

var (
	errParseCert     = errors.New("Failed to parse certificate")
	errParseRootCert = errors.New("Failed to parse root certificate")
)

type Cert struct {
	cert *x509.Certificate
}

func (c *Cert) Validate(pem []byte) error {
	roots := x509.NewCertPool()
	if ok := roots.AppendCertsFromPEM(pem); !ok {
		return errParseRootCert
	}

	if _, err := c.cert.Verify(x509.VerifyOptions{Roots: roots}); err != nil {
		return err
	}

	return nil
}

func (c *Cert) EqualBytes(bs ...[]byte) bool {
	for _, b := range bs {
		other, err := parseCert(b)
		if err != nil {
			return false
		}

		if c.cert.Equal(other) {
			return true
		}
	}

	return false
}

func parseCert(b []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errParseCert
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	return cert, nil
}

func ParseCert(b []byte) (c *Cert, err error) {
	cert, err := parseCert(b)
	if err != nil {
		return nil, err
	}

	return &Cert{cert}, nil
}
