/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"crypto/rand"
	"fmt"
	mrand "math/rand/v2"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// The suites differ in the length of the block signature and of the hashes in the header
var blockSizeSuites = []struct{ cryptoer, hasher string }{
	{"ECC_P256", "SHA256"},
	{"ECC_Secp256k1", "KECCAK256"},
	{"SM2", "SM3"},
	{"MLDSA65", "SHA3_256"},
	{"MLDSA87", "SHA384"},
	{"MLDSA87", "SHA512"},
}

type testTx struct {
	fullData  []byte
	after     *types.AfterTx
	rollbacks []*types.RollbackTx
}

// newTestTx returns a transaction as play leaves it: its stored bytes, half random so that they
// do not compress away, and the after_txs entries it adds
func newTestTx(r *mrand.Rand, blockID int64) testTx {
	fullData := make([]byte, 200+r.IntN(8000))
	rand.Read(fullData[:len(fullData)/2])
	hash := crypto.DoubleHash(fullData)
	tx := testTx{
		fullData: fullData,
		after: &types.AfterTx{
			UsedTx: hash,
			Lts: &types.LogTransaction{
				Block:        blockID,
				Hash:         hash,
				Timestamp:    r.Int64(),
				Address:      r.Int64(),
				EcosystemId:  1,
				ContractName: "@1TokensTransfer",
				InvokeStatus: pbgo.TxInvokeStatusCode(r.IntN(3)),
			},
			UpdTxStatus: &pbgo.TxResult{Hash: hash, Result: "1", BlockId: blockID},
		},
	}
	for range r.IntN(4) {
		data := make([]byte, r.IntN(400))
		rand.Read(data)
		tx.rollbacks = append(tx.rollbacks, &types.RollbackTx{
			BlockId:   blockID,
			TxHash:    hash,
			NameTable: "1_keys",
			TableId:   fmt.Sprint(r.Int64()),
			Data:      string(data),
			DataHash:  crypto.Hash(data),
		})
	}
	return tx
}

func testHeaders(r *mrand.Rand) (*types.BlockHeader, *types.BlockHeader) {
	prev := &types.BlockHeader{
		BlockId:       41,
		Timestamp:     r.Int64(),
		KeyId:         r.Int64(),
		Sign:          make([]byte, crypto.NodeSignatureSize()),
		BlockHash:     crypto.Hash([]byte("prev")),
		RollbacksHash: crypto.Hash([]byte("prev rollbacks")),
		Version:       4,
		NetworkId:     1,
	}
	cur := &types.BlockHeader{
		BlockId:      42,
		Timestamp:    r.Int64(),
		EcosystemId:  1,
		KeyId:        r.Int64(),
		NodePosition: 2,
		Version:      4,
		NetworkId:    1,
	}
	return cur, prev
}

// marshalTestBlock encodes the block the way the generating node does after play (repeatMarshallBlock)
func marshalTestBlock(t *testing.T, key []byte, cur, prev *types.BlockHeader, txs []testTx, sysUpdate bool) []byte {
	t.Helper()
	header := *cur
	header.RollbacksHash = crypto.Hash([]byte("rollbacks"))
	afters := &types.AfterTxs{}
	var fullData [][]byte
	for _, tx := range txs {
		afters.Txs = append(afters.Txs, tx.after)
		afters.Rts = append(afters.Rts, tx.rollbacks...)
		fullData = append(fullData, bytes.Clone(tx.fullData))
	}
	block := &types.BlockData{}
	if err := block.Apply(types.WithCurHeader(&header), types.WithPrevHeader(prev), types.WithAfterTxs(afters),
		types.WithSysUpdate(sysUpdate), types.WithTxFullData(fullData)); err != nil {
		t.Fatal(err)
	}
	bin, err := block.MarshallBlock(key)
	if err != nil {
		t.Fatal(err)
	}
	return bin
}

// The count never falls short of the encoded block, and exceeds it only by the room left for the
// fields counted at their maximum length: the signature, and sys_update when no transaction set it
func TestBlockSizeCountsEncodedBlock(t *testing.T) {
	for _, s := range blockSizeSuites {
		t.Run(s.cryptoer+"/"+s.hasher, func(t *testing.T) {
			crypto.InitAsymAlgo(s.cryptoer)
			crypto.InitHashAlgo(s.hasher)
			key, _, err := crypto.GenNodeKeyPair()
			if err != nil {
				t.Fatal(err)
			}
			r := mrand.New(mrand.NewPCG(1, 2))
			for _, sysUpdate := range []bool{false, true} {
				cur, prev := testHeaders(r)
				size := newBlockSize(1<<40, cur, prev)
				var txs []testTx
				for n := range 40 {
					tx := newTestTx(r, cur.BlockId)
					if !size.add(tx.fullData, tx.after, tx.rollbacks) {
						t.Fatal("add refused a transaction under the limit")
					}
					txs = append(txs, tx)
					if n%7 != 0 {
						continue
					}
					bin := marshalTestBlock(t, key, cur, prev, txs, sysUpdate)
					slack := int64(crypto.NodeSignatureSize()) + 2
					if got := int64(len(bin)); got > size.size() || size.size()-got > slack {
						t.Fatalf("%d transactions: block is %d bytes, counted %d", len(txs), got, size.size())
					}
				}
			}
		})
	}
}

// A block filled up to max_block_size is one its peers accept; the transaction that would take it
// one byte over is not added
func TestBlockSizeFillsToLimit(t *testing.T) {
	for _, s := range blockSizeSuites {
		t.Run(s.cryptoer+"/"+s.hasher, func(t *testing.T) {
			crypto.InitAsymAlgo(s.cryptoer)
			crypto.InitHashAlgo(s.hasher)
			key, _, err := crypto.GenNodeKeyPair()
			if err != nil {
				t.Fatal(err)
			}
			r := mrand.New(mrand.NewPCG(3, 4))
			cur, prev := testHeaders(r)
			var txs []testTx
			for range 25 {
				txs = append(txs, newTestTx(r, cur.BlockId))
			}
			// The limit at which exactly the 25 transactions fit
			count := newBlockSize(1<<40, cur, prev)
			for _, tx := range txs {
				count.add(tx.fullData, tx.after, tx.rollbacks)
			}
			limit := count.size()

			for _, l := range []struct {
				limit int64
				fit   int
			}{{limit, len(txs)}, {limit - 1, len(txs) - 1}} {
				size := newBlockSize(l.limit, cur, prev)
				var fit []testTx
				for _, tx := range txs {
					before := *size
					if !size.add(tx.fullData, tx.after, tx.rollbacks) {
						if *size != before {
							t.Fatal("a refused transaction was counted")
						}
						break
					}
					fit = append(fit, tx)
				}
				if len(fit) != l.fit {
					t.Fatalf("limit %d: %d transactions fit, want %d", l.limit, len(fit), l.fit)
				}
				bin := marshalTestBlock(t, key, cur, prev, fit, true)
				// The check a peer applies to the block it receives (ProcessBlockByBinData)
				if _, err := types.ParseBlockHeader(bytes.NewBuffer(bin), l.limit); err != nil {
					t.Fatalf("limit %d: peer refuses the block: %v", l.limit, err)
				}
			}
		})
	}
}

// A transaction larger than an empty block is refused even as the first one
func TestBlockSizeRefusesOversizedTransaction(t *testing.T) {
	crypto.InitAsymAlgo("MLDSA87")
	crypto.InitHashAlgo("SHA512")
	r := mrand.New(mrand.NewPCG(5, 6))
	cur, prev := testHeaders(r)
	empty := newBlockSize(1<<40, cur, prev).size()
	tx := newTestTx(r, cur.BlockId)
	size := newBlockSize(empty+int64(len(tx.fullData))/2, cur, prev)
	if size.add(tx.fullData, tx.after, tx.rollbacks) {
		t.Fatal("a transaction larger than the room in an empty block was added")
	}
}
