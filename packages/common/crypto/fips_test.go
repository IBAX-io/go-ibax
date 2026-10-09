/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package crypto

import (
	"crypto/fips140"
	"os"
	"os/exec"
	"strings"
	"testing"
)

var (
	fipsCryptoers    = []string{"ECC_P256", "MLDSA65", "MLDSA87"}
	nonFIPSCryptoers = []string{"ECC_Secp256k1", "SM2", "ECC_P512"}
	fipsHashers      = []string{"SHA256", "SHA384", "SHA512", "SHA3_256"}
	nonFIPSHashers   = []string{"KECCAK256", "SM3"}
)

func TestCheckAlgosByModeAndModule(t *testing.T) {
	for _, mldsa := range []bool{true, false} {
		for _, fips := range []bool{true, false} {
			for _, name := range append(append([]string{}, fipsCryptoers...), nonFIPSCryptoers...) {
				a := AsymAlgo(AsymAlgo_value[name])
				err := checkAsymAlgo(a, fips, mldsa, "v1.0.0")
				want := a != AsymAlgo_ECC_P512 && !(fips && !fipsAsymAlgos[a]) && !(isMLDSA(a) && !mldsa)
				if (err == nil) != want {
					t.Errorf("%s fips=%v mldsa=%v: %v", name, fips, mldsa, err)
				}
			}
			for _, name := range append(append([]string{}, fipsHashers...), nonFIPSHashers...) {
				a := HashAlgo(HashAlgo_value[name])
				err := checkHashAlgo(a, fips)
				if want := !fips || fipsHashAlgos[a]; (err == nil) != want {
					t.Errorf("%s fips=%v: %v", name, fips, err)
				}
			}
		}
	}
}

const initAlgoEnv = "IBAX_TEST_INIT_ALGO"

// A node in FIPS mode refuses to start with an algorithm that is not approved, and says why
func TestInitRefusesNonFIPSAlgosInFIPSMode(t *testing.T) {
	if algo := os.Getenv(initAlgoEnv); algo != "" {
		kind, name, _ := strings.Cut(algo, ":")
		if kind == "asym" {
			InitAsymAlgo(name)
		} else {
			InitHashAlgo(name)
		}
		os.Exit(0)
	}
	run := func(algo string) (string, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestInitRefusesNonFIPSAlgosInFIPSMode$")
		cmd.Env = append(os.Environ(), "GODEBUG=fips140=on", initAlgoEnv+"="+algo)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	refused := map[string]string{"asym:ECC_P512": "not supported yet"}
	for _, n := range nonFIPSCryptoers {
		if n != "ECC_P512" {
			refused["asym:"+n] = "not approved in FIPS 140-3 mode"
		}
	}
	for _, n := range nonFIPSHashers {
		refused["hash:"+n] = "not approved in FIPS 140-3 mode"
	}
	var allowed []string
	for _, n := range fipsHashers {
		allowed = append(allowed, "hash:"+n)
	}
	allowed = append(allowed, "asym:ECC_P256")
	for algo, why := range refused {
		out, err := run(algo)
		if err == nil || !strings.Contains(out, why) {
			t.Errorf("%s: started in FIPS mode: %v\n%s", algo, err, out)
		}
	}
	for _, algo := range allowed {
		if out, err := run(algo); err != nil {
			t.Errorf("%s: refused in FIPS mode: %v\n%s", algo, err, out)
		}
	}
}

// TestFIPSSuites runs only in FIPS mode (the crypto workflow builds with GOFIPS140 and runs with
// GODEBUG=fips140=only): every approved suite the module implements signs, verifies, hashes and
// MACs, and HMAC refuses keys FIPS does not approve
func TestFIPSSuites(t *testing.T) {
	if !FIPSMode() {
		t.Skip("not in FIPS 140-3 mode")
	}
	t.Logf("Go Cryptographic Module %s, enforced %v", FIPSModule(), fips140.Enforced())
	defer func() { asymAlgo, hashAlgo = AsymAlgo_ECC_P256, HashAlgo_SHA256 }()
	for _, cryptoer := range fipsCryptoers {
		a := AsymAlgo(AsymAlgo_value[cryptoer])
		if err := CheckAsymAlgo(a); err != nil {
			if isMLDSA(a) {
				t.Logf("%s: %v", cryptoer, err)
				continue
			}
			t.Fatal(err)
		}
		for _, hasher := range fipsHashers {
			InitAsymAlgo(cryptoer)
			InitHashAlgo(hasher)
			priv, pub, err := GenKeyPair()
			if err != nil {
				t.Fatalf("%s/%s: %v", cryptoer, hasher, err)
			}
			msg := []byte("fips " + cryptoer + " " + hasher)
			sig, err := Sign(priv, msg)
			if err != nil {
				t.Fatalf("%s/%s: %v", cryptoer, hasher, err)
			}
			if ok, err := Verify(pub, msg, sig); err != nil || !ok {
				t.Errorf("%s/%s: verify %v %v", cryptoer, hasher, ok, err)
			}
			if len(Hash(msg)) != HashSize() || len(DoubleHash(msg)) != HashSize() {
				t.Errorf("%s/%s: hash size", cryptoer, hasher)
			}
			if _, err := GetHMAC(strings.Repeat("k", fipsMinHMACKeySize), "m"); err != nil {
				t.Errorf("%s/%s: HMAC: %v", cryptoer, hasher, err)
			}
			if _, err := GetHMAC(strings.Repeat("k", fipsMinHMACKeySize-1), "m"); err == nil {
				t.Errorf("%s/%s: HMAC accepted a %d byte key", cryptoer, hasher, fipsMinHMACKeySize-1)
			}
		}
	}
}
