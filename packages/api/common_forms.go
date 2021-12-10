/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

const (
	defaultPaginatorLimit = 25
	maxPaginatorLimit     = 1000
)

type paginatorForm struct {
	defaultLimit int

	Limit  int `schema:"limit"`
	Offset int `schema:"offset"`
}

func (f *paginatorForm) Validate(r *http.Request) error {
	if f.Limit <= 0 {
		f.Limit = f.defaultLimit
		if f.Limit == 0 {
			f.Limit = defaultPaginatorLimit
		}
	}

	if f.Limit > maxPaginatorLimit {
		f.Limit = maxPaginatorLimit
	}

	return nil
}

type paramsForm struct {
	nopeValidator
	Names string `schema:"names"`
}

func (f *paramsForm) AcceptNames() map[string]bool {
	names := make(map[string]bool)
	for _, item := range strings.Split(f.Names, ",") {
		if len(item) == 0 {
			continue
		}
		names[item] = true
	}
	return names
}

type ecosystemForm struct {
	EcosystemID     int64  `schema:"ecosystem"`
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
		if err == ErrEcosystemNotFound {
			err = errEcosystem.Errorf(f.EcosystemID)
		}
		return err
	}

	f.EcosystemID = ecosysID
	f.EcosystemPrefix = converter.Int64ToStr(f.EcosystemID)

	return nil
}
