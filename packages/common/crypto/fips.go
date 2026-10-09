/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package crypto

import (
	"crypto/fips140"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/common/crypto/asymalgo"
)

// The algorithms a node accepts in FIPS 140-3 mode (GODEBUG=fips140=on or only): those the Go
// Cryptographic Module implements as approved services. ECC_Secp256k1, SM2, SM3 and KECCAK256 are
// not FIPS algorithms; Keccak-256 is not FIPS 202 SHA3-256.
var (
	fipsAsymAlgos = map[AsymAlgo]bool{AsymAlgo_ECC_P256: true, AsymAlgo_MLDSA65: true, AsymAlgo_MLDSA87: true}
	fipsHashAlgos = map[HashAlgo]bool{HashAlgo_SHA256: true, HashAlgo_SHA384: true, HashAlgo_SHA512: true, HashAlgo_SHA3_256: true}
)

// FIPSMode reports whether the node runs in FIPS 140-3 mode
func FIPSMode() bool {
	return fips140.Enabled()
}

// FIPSModule is the version of the Go Cryptographic Module the node is built with: a frozen
// module such as v1.0.0 when built with GOFIPS140, otherwise "latest"
func FIPSModule() string {
	return fips140.Version()
}

func isMLDSA(a AsymAlgo) bool {
	return a == AsymAlgo_MLDSA65 || a == AsymAlgo_MLDSA87
}

// CheckAsymAlgo refuses a signature algorithm this node cannot run (InitAsymAlgo stops the node on it)
func CheckAsymAlgo(a AsymAlgo) error {
	return checkAsymAlgo(a, FIPSMode(), asymalgo.MLDSAAvailable(), FIPSModule())
}

// CheckHashAlgo refuses a hash algorithm this node cannot run (InitHashAlgo stops the node on it)
func CheckHashAlgo(a HashAlgo) error {
	return checkHashAlgo(a, FIPSMode())
}

// checkAsymAlgo refuses a signature algorithm the node cannot run: one it does not implement, one
// that is not approved in FIPS mode, or ML-DSA when the module the node is built with lacks it
func checkAsymAlgo(a AsymAlgo, fips, mldsa bool, module string) error {
	if a == AsymAlgo_ECC_P512 {
		return fmt.Errorf("curve algo [%v] is not supported yet, Run 'go-ibax config --help' for details", a)
	}
	if fips && !fipsAsymAlgos[a] {
		return fmt.Errorf("curve algo [%v] is not approved in FIPS 140-3 mode; a FIPS node signs with ECC_P256, MLDSA65 or MLDSA87", a)
	}
	if isMLDSA(a) && !mldsa {
		return fmt.Errorf("curve algo [%v] is not implemented by the Go Cryptographic Module %s this node is built with; build it with a module that has ML-DSA (GOFIPS140=v1.26.0 or later)", a, module)
	}
	return nil
}

// checkHashAlgo refuses a hash algorithm that is not approved in FIPS mode
func checkHashAlgo(a HashAlgo, fips bool) error {
	if fips && !fipsHashAlgos[a] {
		return fmt.Errorf("hash algo [%v] is not approved in FIPS 140-3 mode; a FIPS node hashes with SHA256, SHA384, SHA512 or SHA3_256", a)
	}
	return nil
}

// fipsMinHMACKeySize is the shortest HMAC key FIPS 140-3 approves: 112 bits (SP 800-131A)
const fipsMinHMACKeySize = 112 / 8

// CheckHMACKey refuses, in FIPS mode, an HMAC key shorter than FIPS approves; with
// GODEBUG=fips140=only, crypto/hmac would panic on it
func CheckHMACKey(size int) error {
	return checkHMACKey(size, FIPSMode())
}

func checkHMACKey(size int, fips bool) error {
	if fips && size < fipsMinHMACKeySize {
		return fmt.Errorf("HMAC key of %d bytes is not approved in FIPS 140-3 mode: at least %d bytes are required", size, fipsMinHMACKeySize)
	}
	return nil
}
