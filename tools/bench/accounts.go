/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/converter"
)

// account is a key that signs transactions: its algorithm, its private key, its account public key
// and its account
type account struct {
	algo    crypto.AsymAlgo
	private []byte
	public  crypto.AccountKey
	keyID   int64
}

func (a *account) address() string { return converter.AddressToString(a.keyID) }

func newAccount(algo crypto.AsymAlgo, private []byte) (*account, error) {
	public, err := crypto.AccountKeyOf(algo, private)
	if err != nil {
		return nil, err
	}
	return &account{algo: algo, private: private, public: public, keyID: public.Address()}, nil
}

// readKey reads a private key in hex, of the algorithm named, from a file
func readKey(file, algoName string) (*account, error) {
	algo, err := parseAlgo(algoName)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	private, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return newAccount(algo, private)
}

func parseAlgo(name string) (crypto.AsymAlgo, error) {
	v, ok := crypto.AsymAlgo_value[name]
	if !ok || !crypto.AccountAlgoImplemented(crypto.AsymAlgo(v)) {
		return 0, fmt.Errorf("%q is no account algorithm go-ibax implements", name)
	}
	return crypto.AsymAlgo(v), nil
}

// accountsFile is the file of the accounts of the bench: the private keys, of the algorithm named
type accountsFile struct {
	Algo     string   `json:"algo"`
	Privates []string `json:"privates"`
}

// genAccounts makes n accounts of an algorithm
func genAccounts(algoName string, n int) ([]*account, error) {
	algo, err := parseAlgo(algoName)
	if err != nil {
		return nil, err
	}
	out := make([]*account, 0, n)
	for range n {
		private, _, err := crypto.GenAccountKey(algo)
		if err != nil {
			return nil, err
		}
		a, err := newAccount(algo, private)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func saveAccounts(file string, accounts []*account) error {
	f := accountsFile{Algo: accounts[0].algo.String()}
	for _, a := range accounts {
		f.Privates = append(f.Privates, hex.EncodeToString(a.private))
	}
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(file, b, 0o600)
}

func loadAccounts(file string) ([]*account, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var f accountsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	algo, err := parseAlgo(f.Algo)
	if err != nil {
		return nil, err
	}
	out := make([]*account, 0, len(f.Privates))
	for _, p := range f.Privates {
		private, err := hex.DecodeString(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		a, err := newAccount(algo, private)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s has no account", file)
	}
	return out, nil
}
