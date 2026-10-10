/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
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
	return crypto.CheckAsymAlgo(crypto.AsymAlgo(crypto.AsymAlgo_value[cryptoer])) == nil &&
		crypto.CheckHashAlgo(crypto.HashAlgo(crypto.HashAlgo_value[hasher])) == nil
}

func useSuite(cryptoer, hasher string) {
	conf.Config.CryptoSettings.Cryptoer, conf.Config.CryptoSettings.Hasher = cryptoer, hasher
	crypto.InitAsymAlgo(cryptoer)
	crypto.InitHashAlgo(hasher)
}

// testGenesis makes a genesis block as generateFirstBlock does, under the current suite
func testGenesis(t *testing.T) []byte {
	t.Helper()
	priv, pub, err := crypto.GenNodeKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := new(transaction.FirstBlockParser).BinMarshal(&types.FirstBlock{
		KeyID: 1, Time: 1, PublicKey: pub, NodePublicKey: pub,
	})
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
