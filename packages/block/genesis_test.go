/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"fmt"
	"strings"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// usable reports whether this node can run the suite: in FIPS mode only approved suites, and
// ML-DSA only with a module that implements it
func usable(cryptoer, hasher string) bool {
	return crypto.CheckNodeAlgo(crypto.AsymAlgo(crypto.AsymAlgo_value[cryptoer])) == nil &&
		crypto.CheckHashAlgo(crypto.HashAlgo(crypto.HashAlgo_value[hasher])) == nil
}

func useSuite(cryptoer, hasher string) {
	conf.Config.CryptoSettings.Cryptoer, conf.Config.CryptoSettings.Hasher = cryptoer, hasher
	crypto.InitAsymAlgo(cryptoer)
	crypto.InitHashAlgo(hasher)
}

// testGenesis makes a genesis block as generateFirstBlock does, under the current suite: the
// founder has an account key of the node algorithm, the only account algorithm. edit, if any,
// changes the first block before it is signed.
// genesisTime is 2030-06-15 12:00 UTC
const genesisTime = 1907755200

func testGenesis(t *testing.T, edit ...func(*types.FirstBlock)) []byte {
	t.Helper()
	priv, pub, err := crypto.GenNodeKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_, founder, err := crypto.GenAccountKey(crypto.NodeAlgo())
	if err != nil {
		t.Fatal(err)
	}
	first := &types.FirstBlock{
		KeyID: founder.Address(), Time: genesisTime, PublicKey: founder.Bytes(), NodePublicKey: pub,
		AccountAlgorithms: fmt.Sprintf(`[{"algo":%q}]`, crypto.NodeAlgo()),
	}
	for _, e := range edit {
		e(first)
	}
	tx, err := new(transaction.FirstBlockParser).BinMarshal(first)
	if err != nil {
		t.Fatal(err)
	}
	b := &types.BlockData{
		Header: &types.BlockHeader{
			BlockId: 1, Timestamp: 1, KeyId: 1, Version: consts.BlockVersion,
			RollbacksHash: crypto.Hash([]byte(`0`)), ConsensusMode: consts.HonorNodeMode,
		},
		PrevHeader: &types.BlockHeader{
			BlockHash: crypto.DoubleHash([]byte(`0`)), RollbacksHash: crypto.Hash([]byte(`0`)),
		},
		TxFullData: [][]byte{tx},
	}
	data, err := b.MarshallBlock(priv)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// Every hasher with ECC_P256, so that a module without ML-DSA still checks four FIPS suites
var genesisSuites = append([]struct{ cryptoer, hasher string }{
	{"ECC_P256", "SHA384"}, {"ECC_P256", "SHA512"}, {"ECC_P256", "SHA3_256"},
}, blockSizeSuites...)

func TestCheckGenesisRefusesAnotherSuite(t *testing.T) {
	defer useSuite("ECC_P256", "SHA256")
	for _, made := range genesisSuites {
		if !usable(made.cryptoer, made.hasher) {
			continue
		}
		useSuite(made.cryptoer, made.hasher)
		genesis := testGenesis(t)
		if err := CheckGenesis(genesis); err != nil {
			t.Errorf("%s/%s genesis refused under its own suite: %v", made.cryptoer, made.hasher, err)
		}
		for _, run := range genesisSuites {
			if run == made || !usable(run.cryptoer, run.hasher) {
				continue
			}
			useSuite(run.cryptoer, run.hasher)
			err := CheckGenesis(genesis)
			if err == nil || !strings.Contains(err.Error(), "does not match the configured crypto suite") {
				t.Errorf("%s/%s genesis checked under %s/%s: %v", made.cryptoer, made.hasher, run.cryptoer, run.hasher, err)
			}
		}
	}
}

// Same hasher, another cryptoer: only the signature tells the suites apart
func TestCheckGenesisRefusesAnotherCryptoer(t *testing.T) {
	defer useSuite("ECC_P256", "SHA256")
	for _, pair := range [][2]string{{"ECC_P256", "ECC_Secp256k1"}, {"ECC_P256", "SM2"}, {"MLDSA65", "MLDSA87"}, {"MLDSA87", "ECC_P256"}} {
		if !usable(pair[0], "SHA256") || !usable(pair[1], "SHA256") {
			continue
		}
		useSuite(pair[0], "SHA256")
		genesis := testGenesis(t)
		useSuite(pair[1], "SHA256")
		if err := CheckGenesis(genesis); err == nil || !strings.Contains(err.Error(), "signature does not verify") {
			t.Errorf("%s genesis checked under %s: %v", pair[0], pair[1], err)
		}
	}
}

func TestCheckGenesisRefusesAlteredBlock(t *testing.T) {
	defer useSuite("ECC_P256", "SHA256")
	useSuite("ECC_P256", "SHA256")
	b := &types.BlockData{}
	if err := b.UnmarshallBlock(testGenesis(t)); err != nil {
		t.Fatal(err)
	}
	b.Header.Timestamp++
	b.Header.BlockHash = b.Header.GenHash(b.PrevHeader, b.MerkleRoot)
	b.TxFullData[0] = types.DoZlibCompress(b.TxFullData[0])
	data, err := b.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckGenesis(data); err == nil || !strings.Contains(err.Error(), "signature does not verify") {
		t.Errorf("altered genesis: %v", err)
	}
}

// The genesis block must hold a valid account_algorithms the node algorithm is in, and a founder
// account key that may be registered at the genesis time
func TestCheckGenesisRefusesBadAccounts(t *testing.T) {
	defer useSuite("ECC_P256", "SHA256")
	useSuite("ECC_P256", "SHA256")
	// Any ML-DSA-65 key: under a module without ML-DSA, the bytes stand for one and are refused
	mldsa := crypto.AccountKey{Algo: crypto.AsymAlgo_MLDSA65, Raw: make([]byte, 1952)}
	withMLDSA := crypto.CheckAccountAlgo(crypto.AsymAlgo_MLDSA65) == nil
	if withMLDSA {
		var err error
		if _, mldsa, err = crypto.GenAccountKey(crypto.AsymAlgo_MLDSA65); err != nil {
			t.Fatal(err)
		}
	}
	cases := map[string]func(*types.FirstBlock){
		"no account algorithms": func(f *types.FirstBlock) { f.AccountAlgorithms = "" },
		"empty set":             func(f *types.FirstBlock) { f.AccountAlgorithms = "[]" },
		"node algorithm missing": func(f *types.FirstBlock) {
			f.AccountAlgorithms = `[{"algo":"MLDSA65"}]`
		},
		"node algorithm past its signing day": func(f *types.FirstBlock) {
			f.AccountAlgorithms = `[{"algo":"ECC_P256","sign_until":"2030-06-14"}]`
		},
		"founder algorithm missing": func(f *types.FirstBlock) {
			f.PublicKey, f.KeyID = mldsa.Bytes(), mldsa.Address()
		},
		"founder past its registration day": func(f *types.FirstBlock) {
			f.AccountAlgorithms = `[{"algo":"ECC_P256","register_until":"2030-06-14"}]`
		},
		"bare founder key":               func(f *types.FirstBlock) { f.PublicKey = f.PublicKey[2:] },
		"founder key of another account": func(f *types.FirstBlock) { f.KeyID++ },
	}
	for name, edit := range cases {
		if err := CheckGenesis(testGenesis(t, edit)); err == nil || !strings.Contains(err.Error(), "the accounts of the genesis block") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// The registration and signing days include the genesis day; a founder of another algorithm
	// in the set is accepted
	accepted := map[string]func(*types.FirstBlock){
		"days on the genesis day": func(f *types.FirstBlock) {
			f.AccountAlgorithms = `[{"algo":"ECC_P256","register_until":"2030-06-15","sign_until":"2030-06-15"}]`
		},
	}
	if withMLDSA {
		accepted["ML-DSA founder"] = func(f *types.FirstBlock) {
			f.AccountAlgorithms = `[{"algo":"ECC_P256"},{"algo":"MLDSA65"}]`
			f.PublicKey, f.KeyID = mldsa.Bytes(), mldsa.Address()
		}
	}
	for name, edit := range accepted {
		if err := CheckGenesis(testGenesis(t, edit)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A FIPS node refuses a genesis block whose accounts may have keys it cannot verify in FIPS mode
func TestCheckGenesisFIPSRefusesAccountAlgorithms(t *testing.T) {
	if !crypto.FIPSMode() {
		t.Skip("runs in FIPS mode (GODEBUG=fips140=on)")
	}
	defer useSuite("ECC_P256", "SHA256")
	useSuite("ECC_P256", "SHA256")
	err := CheckGenesis(testGenesis(t, func(f *types.FirstBlock) {
		f.AccountAlgorithms = `[{"algo":"ECC_P256"},{"algo":"ECC_Secp256k1"}]`
	}))
	if err == nil || !strings.Contains(err.Error(), "FIPS") {
		t.Errorf("secp256k1 accounts accepted in FIPS mode: %v", err)
	}
}
