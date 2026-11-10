/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"errors"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// TestSunsetAtRegistration checks the last days of the default set of a network of ML-DSA-65
// nodes where keys are registered: a key bound to an account by a contract (HexToPub, as
// @1NewUser and @1BindWallet do) and a key a new account sends with its first transaction or login
// (Signer). RSA-2048 keys are registered until 2030-12-31, the other classical keys until they stop
// signing, 2031-12-31; ML-DSA-65 keys have no last day. The block time decides, at the edge of the
// day in UTC.
func TestSunsetAtRegistration(t *testing.T) {
	crypto.InitAsymAlgo(crypto.AsymAlgo_MLDSA65.String())
	crypto.InitHashAlgo(crypto.HashAlgo_SHA256.String())
	set := syspar.DefaultAccountAlgorithms(crypto.NodeAlgo())
	saved := syspar.GetAccountAlgorithms()
	syspar.SetAccountAlgorithms(set)
	t.Cleanup(func() { syspar.SetAccountAlgorithms(saved) })

	keys := map[crypto.AsymAlgo]crypto.AccountKey{}
	for _, algo := range []crypto.AsymAlgo{crypto.AsymAlgo_MLDSA65, crypto.AsymAlgo_ECC_P256, crypto.AsymAlgo_RSA2048, crypto.AsymAlgo_RSA3072} {
		_, key, err := crypto.GenAccountKey(algo)
		if err != nil {
			t.Fatal(err)
		}
		keys[algo] = key
	}
	at := func(day string, clock time.Duration) int64 {
		d, err := time.Parse(time.DateOnly, day)
		if err != nil {
			t.Fatal(err)
		}
		return d.Add(clock).Unix()
	}
	lastSecond := 24*time.Hour - time.Second
	for _, c := range []struct {
		algo       crypto.AsymAlgo
		time       int64
		registered bool
	}{
		{crypto.AsymAlgo_RSA2048, at("2030-12-31", lastSecond), true},
		{crypto.AsymAlgo_RSA2048, at("2031-01-01", 0), false},
		{crypto.AsymAlgo_RSA3072, at("2031-01-01", 0), true},
		{crypto.AsymAlgo_RSA3072, at("2031-12-31", lastSecond), true},
		{crypto.AsymAlgo_RSA3072, at("2032-01-01", 0), false},
		{crypto.AsymAlgo_ECC_P256, at("2031-12-31", lastSecond), true},
		{crypto.AsymAlgo_ECC_P256, at("2032-01-01", 0), false},
		{crypto.AsymAlgo_MLDSA65, at("2032-01-01", 0), true},
		{crypto.AsymAlgo_MLDSA65, at("2100-01-01", 0), true},
	} {
		key := keys[c.algo]
		day := time.Unix(c.time, 0).UTC().Format(time.DateTime)
		sc := &SmartContract{BlockHeader: &types.BlockHeader{Timestamp: c.time}}
		pub, err := HexToPub(sc, key.Hex())
		if c.registered && (err != nil || len(pub) == 0) {
			t.Errorf("%s bound at %s: %v", c.algo, day, err)
		}
		if !c.registered && !errors.Is(err, syspar.ErrAccountAlgorithm) {
			t.Errorf("%s bound at %s: got %v, want %v", c.algo, day, err, syspar.ErrAccountAlgorithm)
		}
		signer, err := set.Signer(nil, key.Bytes(), key.Address(), c.time)
		if c.registered && (err != nil || signer.Hex() != key.Hex()) {
			t.Errorf("%s sent by a new account at %s: %v", c.algo, day, err)
		}
		if !c.registered && !errors.Is(err, syspar.ErrAccountAlgorithm) {
			t.Errorf("%s sent by a new account at %s: got %v, want %v", c.algo, day, err, syspar.ErrAccountAlgorithm)
		}
	}
}
