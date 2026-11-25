/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// testDecodedBlock is the bytes and the hash of a block of the height of n transfers of the UTXO
// model, which decode without a contract
func testDecodedBlock(t *testing.T, height int64, n int) ([]byte, bft.Hash) {
	t.Helper()
	crypto.InitHashAlgo("SHA3_256")
	type stored struct {
		_msgpack    struct{} `msgpack:",as_array"`
		Payload     []byte
		TxSignature []byte
		Timestamp   int64
	}
	data := &types.BlockData{
		Header:     &types.BlockHeader{BlockId: height, Timestamp: 1_700_000_000 + height, Version: 1},
		PrevHeader: &types.BlockHeader{BlockId: height - 1, BlockHash: []byte("the block before")},
	}
	for i := range n {
		tx := &types.SmartTransaction{
			Header:       &types.Header{EcosystemID: 1, KeyID: int64(i + 1), Time: 1_700_000_000, NetworkID: 1},
			TransferSelf: &types.TransferSelf{Value: "1", Source: "UTXO", Target: "Account"},
		}
		payload, err := msgpack.Marshal(tx)
		require.NoError(t, err)
		body, err := msgpack.Marshal(&stored{Payload: payload, TxSignature: []byte{1}, Timestamp: 1_700_000_000_000})
		require.NoError(t, err)
		data.TxFullData = append(data.TxFullData, types.DoZlibCompress(append([]byte{tx.TxType()}, body...)))
	}
	data.MerkleRoot = data.GenMerkleRoot()
	data.Header.BlockHash = data.Header.GenHash(data.PrevHeader, data.MerkleRoot)
	bin, err := proto.Marshal(data)
	require.NoError(t, err)
	hash, err := bft.HashOf(data.Header.BlockHash)
	require.NoError(t, err)
	return bin, hash
}

// A block decodes with its transactions uncompressed and not bound, only of its hash, of its
// merkle root and of the hash of its content
func TestDecodeBlock(t *testing.T) {
	bin, hash := testDecodedBlock(t, 7, 300)
	e, err := decodeBlock(bin, hash)
	require.NoError(t, err)
	require.EqualValues(t, 7, e.height())
	require.Len(t, e.txs, 300)
	for i, tx := range e.txs {
		require.Equal(t, e.data.TxFullData[i], tx.FullData)
		require.Nil(t, tx.SmartContract().TxContract)
	}

	other := hash
	other[0] ^= 1
	_, err = decodeBlock(bin, other)
	require.ErrorContains(t, err, "is not")

	raw := &types.BlockData{}
	require.NoError(t, proto.Unmarshal(bin, raw))
	raw.TxFullData = raw.TxFullData[1:]
	cut, err := proto.Marshal(raw)
	require.NoError(t, err)
	_, err = decodeBlock(cut, hash)
	require.ErrorContains(t, err, "merkle root")

	raw.TxFullData = nil
	raw.MerkleRoot = raw.GenMerkleRoot()
	cut, err = proto.Marshal(raw)
	require.NoError(t, err)
	_, err = decodeBlock(cut, hash)
	require.ErrorContains(t, err, "not that of its content")
}

// The node keeps a block parsed from the same bytes only, at most decodedKeep of them, the lowest
// heights left out first; the executor takes a block once it is committed, and the blocks of its
// height and below are left out
func TestDecodedBlocks(t *testing.T) {
	d := &decodedBlocks{}
	entries := make(map[int64]*decodedBlock)
	for h := int64(1); h <= decodedKeep+2; h++ {
		bin, hash := testDecodedBlock(t, h, 1)
		e, err := decodeBlock(bin, hash)
		require.NoError(t, err)
		entries[h] = e
		d.put(e)
	}
	require.Len(t, d.blocks, decodedKeep)
	require.Nil(t, d.get(entries[1].hash, entries[1].bin), "the lowest height is left out")
	require.Nil(t, d.get(entries[2].hash, entries[2].bin))
	e := entries[5]
	require.Same(t, e, d.get(e.hash, e.bin))
	require.Nil(t, d.get(e.hash, append([]byte{0}, e.bin...)), "other bytes of the same hash")
	d.put(e)
	require.Len(t, d.blocks, decodedKeep, "a block kept is kept once")

	require.Nil(t, d.take(5), "a block not committed")
	require.Nil(t, d.get(e.hash, e.bin), "taking a height leaves its blocks out")
	d.commit(entries[6].hash)
	require.Same(t, entries[6], d.take(6))
	require.Nil(t, d.take(6))
	require.Len(t, d.blocks, decodedKeep+2-6)
}
