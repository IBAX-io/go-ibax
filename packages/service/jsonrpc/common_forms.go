/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

const (
	defaultPaginatorLimit = 25
	maxPaginatorLimit     = 100
)

type paginatorForm struct {
	defaultLimit int

	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func (f *paginatorForm) Validate(r *http.Request) error {
	if f == nil {
		return errors.New(paramsEmpty)
	}
	f.defaultLimit = defaultPaginatorLimit
	if f.Limit <= 0 {
		f.Limit = f.defaultLimit
	}

	if f.Limit > maxPaginatorLimit {
		f.Limit = maxPaginatorLimit
	}

	return nil
}

type paramsForm struct {
	nopeValidator
	Names []string `schema:"names"`
}

type nopeValidator struct{}

func (np nopeValidator) Validate(r *http.Request) error {
	return nil
}

func (f *paramsForm) AcceptNames(names string) {
	if names != "" {
		f.Names = strings.Split(names, ",")
	}
}

type ecosystemForm struct {
	EcosystemID     int64  `json:"ecosystem"`
	EcosystemPrefix string `schema:"-"`
	Validator       types.EcosystemGetter
}

func (f *ecosystemForm) Validate(r *http.Request) error {
	if f.Validator == nil {
		panic("ecosystemForm.Validator should not be empty")
	}

	client := getClient(r)
	logger := getLogger(r)

	ecosysID, err := f.Validator.ValidateId(f.EcosystemID, client.EcosystemID, logger)
	if err != nil {
		if err.Error() == "Ecosystem not found" {
			err = fmt.Errorf("ecosystem %d doesn't exist", f.EcosystemID)
		}
		return err
	}

	f.EcosystemID = ecosysID
	f.EcosystemPrefix = converter.Int64ToStr(f.EcosystemID)

	return nil
}
