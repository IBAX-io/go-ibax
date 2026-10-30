/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// TestCheckSignVerifyOnly checks the claim of a new chain signed with the secp256k1 key of an
// account of the old one: the migration contract that account_algorithms names verifies it until
// the last day, no other contract does, and the key's account is the account of the old chain
func TestCheckSignVerifyOnly(t *testing.T) {
	crypto.InitHashAlgo(crypto.HashAlgo_KECCAK256.String())
	set, err := syspar.ParseAccountAlgorithms(`[{"algo":"MLDSA65"},{"algo":"ECC_Secp256k1","verify_until":"2027-12-31","verify_by":"@1MigrationClaim"}]`)
	if err != nil {
		t.Fatal(err)
	}
	saved := syspar.GetAccountAlgorithms()
	syspar.SetAccountAlgorithms(set)
	t.Cleanup(func() { syspar.SetAccountAlgorithms(saved) })

	secp := crypto.NewAsymAlgo(crypto.AsymAlgo_ECC_Secp256k1)
	priv, raw, err := secp.GenKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	claim := "IBAX migration claim,old 1234-5678-9012-3456-7890,new 1111-2222-3333-4444-5555"
	signature, err := secp.Sign(priv, crypto.Hash([]byte(claim)))
	if err != nil {
		t.Fatal(err)
	}
	pub := "ac01" + hex.EncodeToString(raw)
	sign := hex.EncodeToString(signature)
	if PubToID(pub) != crypto.Address(raw) {
		t.Fatal("the account of the key is not its account of the old chain")
	}

	run := func(stack []any, day string, data string) (bool, error) {
		d, _ := time.Parse(time.DateOnly, day)
		sc := &SmartContract{
			BlockHeader: &types.BlockHeader{Timestamp: d.Add(12 * time.Hour).Unix()},
			TxContract:  &Contract{Name: "@1MigrationClaim", StackCont: stack},
		}
		return CheckSign(sc, pub, data, sign)
	}
	claimStack := []any{"@1MigrationClaim"}
	if ok, err := run(claimStack, "2026-11-01", claim); !ok || err != nil {
		t.Fatalf("the migration contract: %v, %v", ok, err)
	}
	if ok, err := run(claimStack, "2027-12-31", claim); !ok || err != nil {
		t.Fatalf("the last day: %v, %v", ok, err)
	}
	if ok, _ := run(claimStack, "2026-11-01", claim+"x"); ok {
		t.Fatal("a claim altered verified")
	}
	for _, c := range []struct {
		name  string
		stack []any
		day   string
	}{
		{"the day after", claimStack, "2028-01-01"},
		{"another contract", []any{"@1BindWallet"}, "2026-11-01"},
		{"a contract the migration contract calls", []any{"@1MigrationClaim", "@1Helper"}, "2026-11-01"},
		{"out of a contract", nil, "2026-11-01"},
	} {
		if ok, err := run(c.stack, c.day, claim); ok || !errors.Is(err, syspar.ErrAccountAlgorithm) {
			t.Errorf("%s: %v, %v", c.name, ok, err)
		}
	}
}
