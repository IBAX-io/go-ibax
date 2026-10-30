/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package syspar

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

// verifyOnly is the set of a new chain whose migration contract checks the claims signed with the
// secp256k1 keys of the old chain
const verifyOnly = `[{"algo":"ECC_P256","register_until":"2030-12-31","sign_until":"2031-12-31"},{"algo":"MLDSA65"},{"algo":"ECC_Secp256k1","verify_until":"2027-12-31","verify_by":"@1MigrationClaim"}]`

// An algorithm that only verifies has its last day and the contract that verifies, nothing else,
// and the set has an algorithm that signs
func TestParseAccountAlgorithmsVerifyOnly(t *testing.T) {
	set := mustParse(t, verifyOnly)
	secp := set[2]
	if !secp.VerifyOnly() || secp.VerifyBy != "@1MigrationClaim" || secp.VerifyUntil.Format(time.DateOnly) != "2027-12-31" ||
		set[0].VerifyOnly() || set[1].VerifyOnly() {
		t.Fatalf("parsed %+v", set)
	}
	if set.String() != verifyOnly {
		t.Fatalf("written %s", set)
	}
	var answer AccountAlgorithmSet
	if err := json.Unmarshal([]byte(verifyOnly), &answer); err != nil || !slices.Equal(answer, set) {
		t.Fatalf("read back %+v, %v", answer, err)
	}
	for _, bad := range []string{
		`[{"algo":"ECC_Secp256k1","verify_until":"2027-12-31","verify_by":"@1MigrationClaim"}]`,
		`[{"algo":"MLDSA65"},{"algo":"ECC_Secp256k1","verify_until":"2027-12-31"}]`,
		`[{"algo":"MLDSA65"},{"algo":"ECC_Secp256k1","verify_by":"@1MigrationClaim"}]`,
		`[{"algo":"MLDSA65"},{"algo":"ECC_Secp256k1","verify_until":"2027-12-31","verify_by":"MigrationClaim"}]`,
		`[{"algo":"MLDSA65"},{"algo":"ECC_Secp256k1","verify_until":"2027-12-31","verify_by":"@0MigrationClaim"}]`,
		`[{"algo":"MLDSA65"},{"algo":"ECC_Secp256k1","verify_until":"2027-12-31","verify_by":"@1Migration Claim"}]`,
		`[{"algo":"MLDSA65"},{"algo":"ECC_Secp256k1","verify_until":"2027-31-12","verify_by":"@1MigrationClaim"}]`,
		`[{"algo":"MLDSA65"},{"algo":"ECC_Secp256k1","sign_until":"2027-12-31","verify_until":"2027-12-31","verify_by":"@1MigrationClaim"}]`,
		`[{"algo":"MLDSA65"},{"algo":"ECC_Secp256k1","register_until":"2027-12-31","verify_until":"2027-12-31","verify_by":"@1MigrationClaim"}]`,
	} {
		if set, err := ParseAccountAlgorithms(bad); err == nil {
			t.Errorf("%s parsed as %v", bad, set)
		}
	}
}

// A key of an algorithm that only verifies never registers or signs; its signatures are checked
// in the contract that verifies alone, until the last day
func TestAccountAlgorithmVerifyOnly(t *testing.T) {
	set := mustParse(t, verifyOnly)
	secp, p256 := crypto.AsymAlgo_ECC_Secp256k1, crypto.AsymAlgo_ECC_P256
	lastSecond := 24*time.Hour - time.Second
	claim := "@1MigrationClaim"
	for _, c := range []struct {
		name     string
		err      error
		accepted bool
	}{
		{"sign", set.CheckSign(secp, unix("2026-11-01", 0)), false},
		{"register", set.CheckRegister(secp, unix("2026-11-01", 0)), false},
		{"verify in the contract", set.CheckVerify(secp, unix("2026-11-01", 0), claim), true},
		{"verify on the last day", set.CheckVerify(secp, unix("2027-12-31", lastSecond), claim), true},
		{"verify the day after", set.CheckVerify(secp, unix("2028-01-01", 0), claim), false},
		{"verify in another contract", set.CheckVerify(secp, unix("2026-11-01", 0), "@1BindWallet"), false},
		{"verify in the contract of another ecosystem", set.CheckVerify(secp, unix("2026-11-01", 0), "@2MigrationClaim"), false},
		{"verify out of a contract", set.CheckVerify(secp, unix("2026-11-01", 0), ""), false},
		{"verify a signing algorithm anywhere", set.CheckVerify(p256, unix("2031-12-31", 0), "@1BindWallet"), true},
		{"verify a signing algorithm past its signing day", set.CheckVerify(p256, unix("2032-01-01", 0), claim), false},
		{"verify an algorithm not in the set", set.CheckVerify(crypto.AsymAlgo_SM2, unix("2026-11-01", 0), claim), false},
	} {
		if c.accepted && c.err != nil {
			t.Errorf("%s: %v", c.name, c.err)
		}
		if !c.accepted && !errors.Is(c.err, ErrAccountAlgorithm) {
			t.Errorf("%s: %v", c.name, c.err)
		}
	}
	_, pub, err := crypto.NewAsymAlgo(secp).GenKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	key, err := crypto.NewAccountKey(secp, pub)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = set.Signer(nil, key.Bytes(), key.Address(), unix("2026-11-01", 0)); !errors.Is(err, ErrAccountAlgorithm) {
		t.Errorf("a key that only verifies registered its account: %v", err)
	}
	if _, err = set.Signer(key.Bytes(), nil, key.Address(), unix("2026-11-01", 0)); !errors.Is(err, ErrAccountAlgorithm) {
		t.Errorf("a registered key that only verifies signed: %v", err)
	}
}

// The last day of verifying only comes earlier; the contract stays, the entry is never removed,
// and the algorithm neither starts signing nor turns from signing to verifying only
func TestAccountAlgorithmChangeVerifyOnly(t *testing.T) {
	current := mustParse(t, verifyOnly)
	head := `[{"algo":"ECC_P256","register_until":"2030-12-31","sign_until":"2031-12-31"},{"algo":"MLDSA65"}`
	for _, c := range []struct {
		name    string
		next    string
		removed []crypto.AsymAlgo
		refused bool
	}{
		{"unchanged", verifyOnly, nil, false},
		{"last day earlier", head + `,{"algo":"ECC_Secp256k1","verify_until":"2027-06-30","verify_by":"@1MigrationClaim"}]`, nil, false},
		{"last day passed", head + `,{"algo":"ECC_Secp256k1","verify_until":"2026-01-01","verify_by":"@1MigrationClaim"}]`, nil, false},
		{"removed", head + `]`, nil, true},
		{"removed with a signing algorithm", `[{"algo":"MLDSA65"}]`, nil, true},
		{"last day later", head + `,{"algo":"ECC_Secp256k1","verify_until":"2028-01-01","verify_by":"@1MigrationClaim"}]`, nil, true},
		{"another contract", head + `,{"algo":"ECC_Secp256k1","verify_until":"2027-12-31","verify_by":"@1BindWallet"}]`, nil, true},
		{"signing", head + `,{"algo":"ECC_Secp256k1","sign_until":"2027-12-31"}]`, nil, true},
		{"a signing algorithm verifying only", `[{"algo":"ECC_P256","verify_until":"2030-12-31","verify_by":"@1MigrationClaim"},{"algo":"MLDSA65"},{"algo":"ECC_Secp256k1","verify_until":"2027-12-31","verify_by":"@1MigrationClaim"}]`, nil, true},
	} {
		removed, err := current.CheckChange(mustParse(t, c.next))
		if c.refused != (err != nil) || !slices.Equal(removed, c.removed) {
			t.Errorf("%s: removed %v, %v", c.name, removed, err)
		}
	}
	added := mustParse(t, head+`,{"algo":"ECC_Secp256k1","verify_until":"2027-12-31","verify_by":"@1MigrationClaim"}]`)
	if _, err := mustParse(t, head+`]`).CheckChange(added); err != nil {
		t.Errorf("an algorithm that only verifies added: %v", err)
	}
	removed, err := current.CheckChange(mustParse(t, head+`,{"algo":"ECC_P384"}]`))
	if err == nil || removed != nil {
		t.Errorf("an algorithm that only verifies replaced: removed %v, %v", removed, err)
	}
}

// Removing an entry that only verifies and adding the algorithm back, in two values, would bring
// it back signing, later or in another contract: the first value is refused, and the entry it
// leaves, inert once its day has passed, refuses each of them
func TestAccountAlgorithmChangeVerifyOnlyComesBack(t *testing.T) {
	head := `[{"algo":"ECC_P256","register_until":"2030-12-31","sign_until":"2031-12-31"},{"algo":"MLDSA65"}`
	back := []string{
		head + `,{"algo":"ECC_Secp256k1"}]`,
		head + `,{"algo":"ECC_Secp256k1","register_until":"2027-12-31","sign_until":"2027-12-31"}]`,
		head + `,{"algo":"ECC_Secp256k1","verify_until":"2030-12-31","verify_by":"@1MigrationClaim"}]`,
		head + `,{"algo":"ECC_Secp256k1","verify_until":"2027-12-31","verify_by":"@1BindWallet"}]`,
	}
	for _, current := range []AccountAlgorithmSet{
		mustParse(t, verifyOnly),
		mustParse(t, head+`,{"algo":"ECC_Secp256k1","verify_until":"2026-01-01","verify_by":"@1MigrationClaim"}]`),
	} {
		if _, err := current.CheckChange(mustParse(t, head+`]`)); err == nil {
			t.Errorf("%s: the entry that only verifies removed", current)
		}
		for _, next := range back {
			if _, err := current.CheckChange(mustParse(t, next)); err == nil {
				t.Errorf("%s: %s taken", current, next)
			}
		}
	}
}

// A FIPS node takes the last day of verifying only as a last day of signing
func TestAccountAlgorithmsFIPSDaysVerifyOnly(t *testing.T) {
	for _, c := range []struct {
		value string
		ok    bool
	}{
		{`[{"algo":"MLDSA65"},{"algo":"ECC_P256","verify_until":"2031-12-31","verify_by":"@1MigrationClaim"}]`, true},
		{`[{"algo":"MLDSA65"},{"algo":"ECC_P256","verify_until":"2032-01-01","verify_by":"@1MigrationClaim"}]`, false},
	} {
		if err := mustParse(t, c.value).checkFIPSDays(); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.value, err)
		}
	}
}
