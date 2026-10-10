package syspar

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

func mustParse(t *testing.T, value string) AccountAlgorithmSet {
	t.Helper()
	set, err := ParseAccountAlgorithms(value)
	if err != nil {
		t.Fatalf("%s: %v", value, err)
	}
	return set
}

func unix(day string, clock time.Duration) int64 {
	d, _ := time.Parse(time.DateOnly, day)
	return d.Add(clock).Unix()
}

func TestParseAccountAlgorithms(t *testing.T) {
	value := `[{"algo":"ECC_P256","register_until":"2030-12-31","sign_until":"2031-12-31"},{"algo":"MLDSA65"}]`
	set := mustParse(t, value)
	if len(set) != 2 || set[0].Algo != crypto.AsymAlgo_ECC_P256 || set[1].Algo != crypto.AsymAlgo_MLDSA65 ||
		set[0].RegisterUntil.Format(time.DateOnly) != "2030-12-31" || !set[1].SignUntil.IsZero() {
		t.Fatalf("parsed %+v", set)
	}
	if set.String() != value {
		t.Fatalf("written %s", set)
	}
	if again := mustParse(t, `[{"algo":"MLDSA87","sign_until":"2031-12-31"}]`); again[0].RegisterUntil.IsZero() == false {
		t.Fatal("a registration day appeared")
	}
	for _, bad := range []string{
		``, `{}`, `[]`, `null`,
		`[{"algo":"RSA2048"}]`,
		`[{"algo":"ECC_P512"}]`,
		`[{"algo":"ecc_p256"}]`,
		`[{"algo":"MLDSA65"},{"algo":"MLDSA65"}]`,
		`[{"algo":"ECC_P256","sign_until":"2031-13-01"}]`,
		`[{"algo":"ECC_P256","register_until":"31.12.2030"}]`,
		`[{"algo":"ECC_P256","register_until":"2032-01-01","sign_until":"2031-12-31"}]`,
	} {
		if set, err := ParseAccountAlgorithms(bad); err == nil {
			t.Errorf("%s parsed as %v", bad, set)
		}
	}
}

// The days include the whole UTC day and are compared with the block time
func TestAccountAlgorithmDays(t *testing.T) {
	set := mustParse(t, `[{"algo":"ECC_P256","register_until":"2030-12-31","sign_until":"2031-12-31"},{"algo":"MLDSA65"},{"algo":"MLDSA87","sign_until":"2031-12-31"}]`)
	p256, mldsa65, mldsa87 := crypto.AsymAlgo_ECC_P256, crypto.AsymAlgo_MLDSA65, crypto.AsymAlgo_MLDSA87
	lastSecond := 24*time.Hour - time.Second
	for _, c := range []struct {
		name     string
		check    func(crypto.AsymAlgo, int64) error
		algo     crypto.AsymAlgo
		time     int64
		accepted bool
	}{
		{"register on the last day", set.CheckRegister, p256, unix("2030-12-31", lastSecond), true},
		{"register the day after", set.CheckRegister, p256, unix("2031-01-01", 0), false},
		{"sign after the registration day", set.CheckSign, p256, unix("2031-06-01", 0), true},
		{"sign on the last day", set.CheckSign, p256, unix("2031-12-31", lastSecond), true},
		{"sign the day after", set.CheckSign, p256, unix("2032-01-01", 0), false},
		{"no days", set.CheckRegister, mldsa65, unix("2099-01-01", 0), true},
		{"registration ends with signing", set.CheckRegister, mldsa87, unix("2032-01-01", 0), false},
		{"register before signing ends", set.CheckRegister, mldsa87, unix("2031-12-31", 0), true},
		{"sign with an algorithm not in the set", set.CheckSign, crypto.AsymAlgo_ECC_Secp256k1, unix("2030-01-01", 0), false},
		{"register an algorithm not in the set", set.CheckRegister, crypto.AsymAlgo_SM2, unix("2030-01-01", 0), false},
	} {
		err := c.check(c.algo, c.time)
		if c.accepted && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if !c.accepted && !errors.Is(err, ErrAccountAlgorithm) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	var none AccountAlgorithmSet
	if err := none.CheckSign(p256, 0); !errors.Is(err, ErrAccountAlgorithm) {
		t.Errorf("an empty set accepted a key: %v", err)
	}
}

// Days may only come earlier; removed algorithms are reported for the key count check
func TestAccountAlgorithmChange(t *testing.T) {
	current := mustParse(t, `[{"algo":"ECC_P256","register_until":"2030-12-31","sign_until":"2031-12-31"},{"algo":"MLDSA65"}]`)
	for _, c := range []struct {
		name    string
		next    string
		removed []crypto.AsymAlgo
		refused bool
	}{
		{"unchanged", `[{"algo":"ECC_P256","register_until":"2030-12-31","sign_until":"2031-12-31"},{"algo":"MLDSA65"}]`, nil, false},
		{"days earlier", `[{"algo":"ECC_P256","register_until":"2029-12-31","sign_until":"2030-12-31"},{"algo":"MLDSA65"}]`, nil, false},
		{"a day where there was none", `[{"algo":"ECC_P256","register_until":"2030-12-31","sign_until":"2031-12-31"},{"algo":"MLDSA65","register_until":"2040-01-01"}]`, nil, false},
		{"an algorithm added", `[{"algo":"ECC_P256","register_until":"2030-12-31","sign_until":"2031-12-31"},{"algo":"MLDSA65"},{"algo":"MLDSA87"}]`, nil, false},
		{"an algorithm removed", `[{"algo":"MLDSA65"}]`, []crypto.AsymAlgo{crypto.AsymAlgo_ECC_P256}, false},
		{"registration day later", `[{"algo":"ECC_P256","register_until":"2031-01-01","sign_until":"2031-12-31"},{"algo":"MLDSA65"}]`, nil, true},
		{"signing day later", `[{"algo":"ECC_P256","register_until":"2030-12-31","sign_until":"2032-12-31"},{"algo":"MLDSA65"}]`, nil, true},
		{"signing day removed", `[{"algo":"ECC_P256","register_until":"2030-12-31"},{"algo":"MLDSA65"}]`, nil, true},
		{"registration day removed", `[{"algo":"ECC_P256","sign_until":"2031-12-31"},{"algo":"MLDSA65"}]`, nil, true},
	} {
		removed, err := current.CheckChange(mustParse(t, c.next))
		if c.refused != (err != nil) || !slices.Equal(removed, c.removed) {
			t.Errorf("%s: removed %v, %v", c.name, removed, err)
		}
	}
}

// A FIPS node cannot verify every algorithm: it refuses a set with one it may not use
func TestAccountAlgorithmsFIPS(t *testing.T) {
	approved := mustParse(t, `[{"algo":"ECC_P256"},{"algo":"MLDSA65"},{"algo":"MLDSA87"}]`)
	if crypto.CheckAsymAlgo(crypto.AsymAlgo_MLDSA65) == nil {
		if err := approved.CheckNode(); err != nil {
			t.Errorf("approved set refused: %v", err)
		}
	}
	other := mustParse(t, `[{"algo":"ECC_P256"},{"algo":"ECC_Secp256k1"}]`)
	err := other.CheckNode()
	if crypto.FIPSMode() == (err == nil) {
		t.Errorf("FIPS mode %v, secp256k1 set: %v", crypto.FIPSMode(), err)
	}
}
