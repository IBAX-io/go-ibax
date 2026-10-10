/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package syspar

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

// AccountAlgorithms is the platform parameter of the algorithms account keys may have
const AccountAlgorithms = `account_algorithms`

// accountAlgoDate is the layout of the dates of account_algorithms: a UTC calendar day
const accountAlgoDate = time.DateOnly

var (
	// ErrAccountAlgorithm is a key whose algorithm the network does not accept for what it is used for
	ErrAccountAlgorithm = errors.New("account key algorithm not accepted")
	// ErrAccountKeyID is a public key whose address is not the account it is sent for
	ErrAccountKeyID = errors.New("key_id is not the address of the public key")
	// ErrNoAccountKey is an account with no registered public key, sent without one
	ErrNoAccountKey = errors.New("public key is undefined")
)

// AccountAlgorithm is an algorithm account keys may have, with the last day keys of it may be
// registered and the last day they may sign. A zero day is no limit. The days are UTC and
// include the whole day; they are compared with the block time, never the clock of the node.
type AccountAlgorithm struct {
	Algo          crypto.AsymAlgo
	RegisterUntil time.Time
	SignUntil     time.Time
}

// AccountAlgorithmSet is the value of account_algorithms. Its JSON is
//
//	[{"algo":"ECC_P256","register_until":"2030-12-31","sign_until":"2031-12-31"},{"algo":"MLDSA65"}]
type AccountAlgorithmSet []AccountAlgorithm

type accountAlgorithmJSON struct {
	Algo          string `json:"algo"`
	RegisterUntil string `json:"register_until,omitempty"`
	SignUntil     string `json:"sign_until,omitempty"`
}

// ParseAccountAlgorithms reads and checks a value of account_algorithms: at least one algorithm,
// each implemented and listed once, with valid days and registration ending no later than signing
func ParseAccountAlgorithms(value string) (AccountAlgorithmSet, error) {
	var items []accountAlgorithmJSON
	if err := json.Unmarshal([]byte(value), &items); err != nil {
		return nil, fmt.Errorf("%s: %w", AccountAlgorithms, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%s: no algorithm", AccountAlgorithms)
	}
	set := make(AccountAlgorithmSet, 0, len(items))
	for _, item := range items {
		v, ok := crypto.AsymAlgo_value[item.Algo]
		if !ok || !crypto.AccountAlgoImplemented(crypto.AsymAlgo(v)) {
			return nil, fmt.Errorf("%s: algorithm %q is not implemented", AccountAlgorithms, item.Algo)
		}
		algo := crypto.AsymAlgo(v)
		if _, dup := set.find(algo); dup {
			return nil, fmt.Errorf("%s: algorithm %s is listed twice", AccountAlgorithms, algo)
		}
		a := AccountAlgorithm{Algo: algo}
		var err error
		if a.RegisterUntil, err = parseAccountAlgoDate(item.RegisterUntil); err != nil {
			return nil, fmt.Errorf("%s: %s register_until: %w", AccountAlgorithms, algo, err)
		}
		if a.SignUntil, err = parseAccountAlgoDate(item.SignUntil); err != nil {
			return nil, fmt.Errorf("%s: %s sign_until: %w", AccountAlgorithms, algo, err)
		}
		if !a.SignUntil.IsZero() && a.RegisterUntil.After(a.SignUntil) {
			return nil, fmt.Errorf("%s: %s registration must end no later than signing", AccountAlgorithms, algo)
		}
		set = append(set, a)
	}
	return set, nil
}

func parseAccountAlgoDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(accountAlgoDate, s)
}

// String is the JSON of the set
func (s AccountAlgorithmSet) String() string {
	items := make([]accountAlgorithmJSON, len(s))
	for i, a := range s {
		items[i].Algo = a.Algo.String()
		if !a.RegisterUntil.IsZero() {
			items[i].RegisterUntil = a.RegisterUntil.Format(accountAlgoDate)
		}
		if !a.SignUntil.IsZero() {
			items[i].SignUntil = a.SignUntil.Format(accountAlgoDate)
		}
	}
	out, _ := json.Marshal(items)
	return string(out)
}

// MarshalJSON writes the set as its parameter value
func (s AccountAlgorithmSet) MarshalJSON() ([]byte, error) {
	return []byte(s.String()), nil
}

func (s AccountAlgorithmSet) find(algo crypto.AsymAlgo) (AccountAlgorithm, bool) {
	for _, a := range s {
		if a.Algo == algo {
			return a, true
		}
	}
	return AccountAlgorithm{}, false
}

// Has reports whether account keys may have the algorithm, whatever the days
func (s AccountAlgorithmSet) Has(algo crypto.AsymAlgo) bool {
	_, ok := s.find(algo)
	return ok
}

// within reports whether the block time, in Unix seconds, falls on or before the day
func within(day time.Time, blockTime int64) bool {
	return day.IsZero() || blockTime < day.AddDate(0, 0, 1).Unix()
}

// CheckSign refuses a key of the algorithm signing at the block time, in Unix seconds: the
// algorithm is not in the set, or its last signing day has passed
func (s AccountAlgorithmSet) CheckSign(algo crypto.AsymAlgo, blockTime int64) error {
	a, ok := s.find(algo)
	if !ok {
		return fmt.Errorf("%w: %s is not in %s", ErrAccountAlgorithm, algo, AccountAlgorithms)
	}
	if !within(a.SignUntil, blockTime) {
		return fmt.Errorf("%w: %s keys sign until %s", ErrAccountAlgorithm, algo, a.SignUntil.Format(accountAlgoDate))
	}
	return nil
}

// CheckRegister refuses registering a key of the algorithm at the block time: it may not sign,
// or its last registration day has passed. Without a registration day, registration ends with
// signing.
func (s AccountAlgorithmSet) CheckRegister(algo crypto.AsymAlgo, blockTime int64) error {
	if err := s.CheckSign(algo, blockTime); err != nil {
		return err
	}
	if a, _ := s.find(algo); !within(a.RegisterUntil, blockTime) {
		return fmt.Errorf("%w: %s keys are registered until %s", ErrAccountAlgorithm, algo, a.RegisterUntil.Format(accountAlgoDate))
	}
	return nil
}

// Signer is the key an account signs with at the block time. The key registered in 1_keys, when
// there is one, is the only key the account signs with, and its algorithm must still sign. An
// account without a registered key signs with the key it sends (in the transaction header, at
// login), which registers it: it must be a key of keyID, of an algorithm that is still registered.
// The algorithm always comes from the key itself, never from what the sender claims.
func (s AccountAlgorithmSet) Signer(registered, sent []byte, keyID, blockTime int64) (crypto.AccountKey, error) {
	if len(registered) > 0 {
		key, err := crypto.ParseAccountKey(registered)
		if err != nil {
			return crypto.AccountKey{}, err
		}
		return key, s.CheckSign(key.Algo, blockTime)
	}
	if len(sent) == 0 {
		return crypto.AccountKey{}, ErrNoAccountKey
	}
	key, err := crypto.ParseAccountKey(sent)
	if err != nil {
		return crypto.AccountKey{}, err
	}
	if key.Address() != keyID {
		return crypto.AccountKey{}, ErrAccountKeyID
	}
	return key, s.CheckRegister(key.Algo, blockTime)
}

// CheckChange refuses a new value that moves a day of an algorithm kept in the set later or
// removes it: the days set at genesis follow the regulations, and may only come earlier. It
// returns the algorithms the new value removes, which must have no keys left.
func (s AccountAlgorithmSet) CheckChange(next AccountAlgorithmSet) ([]crypto.AsymAlgo, error) {
	later := func(prev, day time.Time) bool {
		return !prev.IsZero() && (day.IsZero() || day.After(prev))
	}
	var removed []crypto.AsymAlgo
	for _, prev := range s {
		a, kept := next.find(prev.Algo)
		if !kept {
			removed = append(removed, prev.Algo)
			continue
		}
		if later(prev.RegisterUntil, a.RegisterUntil) || later(prev.SignUntil, a.SignUntil) {
			return nil, fmt.Errorf("%s: the days of %s may only come earlier", AccountAlgorithms, prev.Algo)
		}
	}
	return removed, nil
}

// CheckNode refuses a set the node cannot verify: an algorithm not approved in FIPS mode, or one
// the cryptographic module of the node lacks. A node that cannot verify every account would fork.
func (s AccountAlgorithmSet) CheckNode() error {
	for _, a := range s {
		if err := crypto.CheckAsymAlgo(a.Algo); err != nil {
			return fmt.Errorf("%s: %w", AccountAlgorithms, err)
		}
	}
	return nil
}

// GetAccountAlgorithms is the current value of account_algorithms
func GetAccountAlgorithms() AccountAlgorithmSet {
	mutex.RLock()
	defer mutex.RUnlock()
	return accountAlgos
}

// SetAccountAlgorithms sets the value of account_algorithms where there is no database to read it
// from (tests, tools)
func SetAccountAlgorithms(set AccountAlgorithmSet) {
	mutex.Lock()
	defer mutex.Unlock()
	accountAlgos = set
}
