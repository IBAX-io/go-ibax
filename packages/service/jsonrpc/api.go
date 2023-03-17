/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package jsonrpc

import (
	"encoding/hex"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/types"
	"net/http"
	"strings"
)

const (
	multipartBuf      = 100000 // the buffer size for ParseMultipartForm
	multipartFormData = "multipart/form-data"
)

type Mode struct {
	EcosystemGetter   types.EcosystemGetter
	ContractRunner    types.SmartContractRunner
	ClientTxProcessor types.ClientTxPreprocessor
}

type UserClient struct {
	KeyID         int64
	AccountID     string
	EcosystemID   int64
	EcosystemName string
	RoleID        int64
	IsMobile      bool
}

func (c *UserClient) Prefix() string {
	return converter.Int64ToStr(c.EcosystemID)
}

type formValidator interface {
	Validate(r *http.Request) error
}

func parameterValidator(r *http.Request, f formValidator) (err error) {
	return f.Validate(r)
}

func isMultipartForm(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Content-Type"), multipartFormData)
}

type hexValue struct {
	value []byte
}

func (hv hexValue) Bytes() []byte {
	return hv.value
}

func (hv *hexValue) UnmarshalText(v []byte) (err error) {
	hv.value, err = hex.DecodeString(string(v))
	return
}
