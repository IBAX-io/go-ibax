/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"strconv"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

const addressesSource = "go-ibax converter.AddressToID / IDToAddress (packages/converter/address.go) via tools/cryptovectors"

type addressesFile struct {
	Source string        `json:"source"`
	Cases  []addressCase `json:"cases"`
}

type addressCase struct {
	Input   string `json:"input"`
	ID      string `json:"id"`
	Address string `json:"address"`
}

func updateAddresses(raw []byte) (any, []string, error) {
	var f addressesFile
	if err := decode(raw, &f); err != nil {
		return nil, nil, err
	}
	f.Source = addressesSource
	for i := range f.Cases {
		c := &f.Cases[i]
		id := converter.AddressToID(c.Input)
		c.ID = strconv.FormatInt(id, 10)
		c.Address = converter.IDToAddress(id)
	}
	return f, nil, nil
}
