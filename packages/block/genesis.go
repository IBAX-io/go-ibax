/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// CheckGenesis reports whether the genesis block was made with the algorithms the node is
// configured with: its prev hashes, merkle root and block hash are recomputed with the configured
// hasher, and its signature is verified with the configured cryptoer and the genesis node key.
// A node whose suite differs from its chain's would fork from the first block it makes, so it
// must not start.
func CheckGenesis(data []byte) (err error) {
	// The block may come from a peer: a malformed one must not crash the node
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("reading the genesis block: %v", r)
		}
	}()
	mismatch := func(what string) error {
		return fmt.Errorf("the genesis block does not match the configured crypto suite (cryptoer %s, hasher %s): %s",
			conf.Config.CryptoSettings.Cryptoer, conf.Config.CryptoSettings.Hasher, what)
	}
	// The merkle root covers the transactions as stored, compressed: they are read without
	// UnmarshallBlock, which decompresses them
	b := &types.BlockData{}
	if err = b.Unmarshal(data); err != nil {
		return fmt.Errorf("reading the genesis block: %w", err)
	}
	if b.Header == nil || b.PrevHeader == nil || b.Header.BlockId != 1 || len(b.TxFullData) != 1 {
		return fmt.Errorf("reading the genesis block: not a genesis block")
	}
	if !bytes.Equal(b.PrevHeader.BlockHash, crypto.DoubleHash([]byte(`0`))) ||
		!bytes.Equal(b.PrevHeader.RollbacksHash, crypto.Hash([]byte(`0`))) ||
		!bytes.Equal(b.Header.RollbacksHash, crypto.Hash([]byte(`0`))) {
		return mismatch("its initial hashes differ")
	}
	if !bytes.Equal(b.MerkleRoot, b.GenMerkleRoot()) {
		return mismatch("its merkle root differs")
	}
	if !bytes.Equal(b.Header.BlockHash, b.Header.GenHash(b.PrevHeader, b.MerkleRoot)) {
		return mismatch("its block hash differs")
	}
	tx := bytes.NewBuffer(types.DoZlibUnCompress(b.TxFullData[0]))
	if t, err := tx.ReadByte(); err != nil || t != types.FirstBlockTxType {
		return fmt.Errorf("reading the genesis block: no first block transaction")
	}
	first := &transaction.FirstBlockParser{}
	if err := first.Unmarshal(tx); err != nil || first.Data == nil {
		return fmt.Errorf("reading the genesis block: %v", err)
	}
	if ok, err := crypto.Verify(first.Data.NodePublicKey, []byte(b.ForSign()), b.Header.Sign); err != nil || !ok {
		return mismatch("its signature does not verify")
	}
	return nil
}
