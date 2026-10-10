/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package syspar

import (
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

// AccountAlgorithmKeys is an algorithm of account_algorithms, its days and the number of keys,
// not deleted, that have it in every ecosystem
type AccountAlgorithmKeys struct {
	Algo          string `json:"algo"`
	RegisterUntil string `json:"register_until,omitempty"`
	SignUntil     string `json:"sign_until,omitempty"`
	Keys          int64  `json:"keys"`
}

// AccountAlgorithmsWithKeys is the current account_algorithms with the keys of each algorithm
func AccountAlgorithmsWithKeys() ([]AccountAlgorithmKeys, error) {
	set := GetAccountAlgorithms()
	list := make([]AccountAlgorithmKeys, len(set))
	for i, a := range set {
		list[i].Algo = a.Algo.String()
		if !a.RegisterUntil.IsZero() {
			list[i].RegisterUntil = a.RegisterUntil.Format(accountAlgoDate)
		}
		if !a.SignUntil.IsZero() {
			list[i].SignUntil = a.SignUntil.Format(accountAlgoDate)
		}
		n, err := sqldb.CountKeysOfAlgo(nil, a.Algo)
		if err != nil {
			return nil, err
		}
		list[i].Keys = n
	}
	return list, nil
}

// AccountAlgoKeys is a page of the accounts whose keys have an account algorithm, and how many
// there are: the keys a registrar rebinds before the algorithm stops signing
type AccountAlgoKeys struct {
	Count int64           `json:"count"`
	List  []sqldb.AlgoKey `json:"list"`
}

// ParseAccountAlgo reads the name of an algorithm accounts can hold keys of
func ParseAccountAlgo(name string) (crypto.AsymAlgo, error) {
	v, ok := crypto.AsymAlgo_value[name]
	if !ok || !crypto.AccountAlgoImplemented(crypto.AsymAlgo(v)) {
		return 0, fmt.Errorf("%w: %q is no account algorithm", ErrAccountAlgorithm, name)
	}
	return crypto.AsymAlgo(v), nil
}

// KeysOfAccountAlgo is a page of the accounts whose keys have the algorithm
func KeysOfAccountAlgo(algo crypto.AsymAlgo, offset, limit int) (*AccountAlgoKeys, error) {
	count, err := sqldb.CountKeysOfAlgo(nil, algo)
	if err != nil {
		return nil, err
	}
	list, err := sqldb.KeysOfAlgo(nil, algo, offset, limit)
	if err != nil {
		return nil, err
	}
	return &AccountAlgoKeys{Count: count, List: list}, nil
}
