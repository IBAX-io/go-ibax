/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// The ceilings of the stages a transaction goes through off the state, by the cores: run with
// -cpu 1,4,8,16,32, each benchmark spreads its work over GOMAXPROCS (b.RunParallel), so ns/op
// over the cores is the throughput of the machine at that many cores

// stageTransfers are the stored transfers of the vectors of the algorithms of the chain
func stageTransfers(b *testing.B) map[string][]byte {
	b.Helper()
	var every syspar.AccountAlgorithmSet
	for _, a := range []crypto.AsymAlgo{crypto.AsymAlgo_ECC_P256, crypto.AsymAlgo_MLDSA65} {
		every = append(every, syspar.AccountAlgorithm{Algo: a})
	}
	syspar.SetAccountAlgorithms(every)
	b.Cleanup(func() { syspar.SetAccountAlgorithms(nil) })
	stored := map[string][]byte{}
	for _, c := range loadTransfers(b) {
		if (c.Cryptoer != "ECC_P256" && c.Cryptoer != "MLDSA65") || stored[c.Cryptoer] != nil {
			continue
		}
		crypto.InitAsymAlgo(c.Cryptoer)
		crypto.InitHashAlgo(c.Hasher)
		conf.Config.LocalConf.NetworkID = c.NetworkID
		data, err := hex.DecodeString(c.Data)
		if err != nil {
			b.Fatal(err)
		}
		t := &Transaction{}
		if err := t.UnmarshallClient(bytes.NewBuffer(data), true); err != nil {
			b.Fatalf("%s: %v", c.Cryptoer, err)
		}
		stored[c.Cryptoer] = t.FullData
	}
	return stored
}

// BenchmarkStageVerify is the verification of a signature, without the cache of the node
func BenchmarkStageVerify(b *testing.B) {
	crypto.InitHashAlgo("SHA256")
	msg := make([]byte, 160)
	for _, algo := range []crypto.AsymAlgo{crypto.AsymAlgo_ECC_P256, crypto.AsymAlgo_MLDSA65} {
		b.Run(algo.String(), func(b *testing.B) {
			priv, key, err := crypto.GenAccountKey(algo)
			if err != nil {
				b.Fatal(err)
			}
			sig, err := crypto.AccountSign(algo, priv, msg)
			if err != nil {
				b.Fatal(err)
			}
			hash := crypto.Hash(msg)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				a := crypto.NewAsymAlgo(algo)
				for pb.Next() {
					if ok, err := a.Verify(key.Raw, hash, sig); !ok || err != nil {
						b.Fatal("a signature does not verify", err)
					}
				}
			})
		})
	}
}

// BenchmarkStageParse parses a stored transfer as a block carries it, uncompressed
func BenchmarkStageParse(b *testing.B) {
	for algo, data := range stageTransfers(b) {
		b.Run(algo, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if _, err := UnmarshallTransaction(bytes.NewBuffer(bytes.Clone(data)), true); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkStageZlib compresses a stored transfer as a block stores it, and uncompresses it as a
// validator takes it
func BenchmarkStageZlib(b *testing.B) {
	for algo, data := range stageTransfers(b) {
		packed := types.DoZlibCompress(data)
		b.Run(algo+"/compress", func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					types.DoZlibCompress(data)
				}
			})
		})
		b.Run(algo+"/uncompress", func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					types.DoZlibUnCompress(packed)
				}
			})
		})
	}
}
