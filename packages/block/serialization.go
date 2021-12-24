/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"bytes"
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

// MarshallBlock is marshalling block
func MarshallBlock(header *types.BlockData, trData [][]byte, prev *types.BlockData, key string) ([]byte, error) {
	var mrklArray [][]byte
	var blockDataTx []byte
	var signed []byte
	logger := log.WithFields(log.Fields{"block_id": header.BlockID, "block_hash": header.Hash, "block_time": header.Time, "block_version": header.Version, "block_wallet_id": header.KeyID, "block_state_id": header.EcosystemID})

	for _, tr := range trData {
		mrklArray = append(mrklArray, converter.BinToHex(crypto.DoubleHash(tr)))
		blockDataTx = append(blockDataTx, converter.EncodeLengthPlusData(tr)...)
	}

	if key != "" {
		if len(mrklArray) == 0 {
			mrklArray = append(mrklArray, []byte("0"))
		}
		mrklRoot, err := utils.MerkleTreeRoot(mrklArray)
		if err != nil {
			return nil, err
		}
		signSource := header.ForSign(prev, mrklRoot)
		signed, err = crypto.SignString(key, signSource)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("signing block")
			return nil, err
		}
	}

	buf := new(bytes.Buffer)

	// fill header
	buf.Write(converter.DecToBin(header.Version, 2))
	buf.Write(converter.DecToBin(header.BlockID, 4))
	buf.Write(converter.DecToBin(header.Time, 4))
	buf.Write(converter.DecToBin(header.EcosystemID, 4))
	buf.Write(converter.EncodeLenInt64InPlace(header.KeyID))
	buf.Write(converter.DecToBin(header.NodePosition, 1))
	buf.Write(converter.EncodeLengthPlusData(prev.RollbacksHash))

	// fill signature
	buf.Write(converter.EncodeLengthPlusData(signed))

	// data
	buf.Write(blockDataTx)

	return buf.Bytes(), nil
}

func UnmarshallBlock(blockBuffer *bytes.Buffer, fillData bool) (*Block, error) {
	header, prev, err := types.ParseBlockHeader(blockBuffer, syspar.GetMaxBlockSize())
	if err != nil {
		return nil, err
	}

	logger := log.WithFields(log.Fields{"block_id": header.BlockID, "block_time": header.Time, "block_wallet_id": header.KeyID,
		"block_state_id": header.EcosystemID, "block_hash": header.Hash, "block_version": header.Version})
	transactions := make([]*transaction.Transaction, 0)

	var mrklSlice [][]byte

	// parse transactions
	for blockBuffer.Len() > 0 {
		transactionSize, err := converter.DecodeLengthBuf(blockBuffer)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.UnmarshallingError, "error": err}).Error("transaction size is 0")
			return nil, fmt.Errorf("bad block format (%s)", err)
		}
		if blockBuffer.Len() < transactionSize {
			logger.WithFields(log.Fields{"size": blockBuffer.Len(), "match_size": int(transactionSize), "type": consts.SizeDoesNotMatch}).Error("transaction size does not matches encoded length")
			return nil, fmt.Errorf("bad block format (transaction len is too big: %d)", transactionSize)
		}

		if transactionSize == 0 {
			logger.WithFields(log.Fields{"type": consts.EmptyObject}).Error("transaction size is 0")
			return nil, fmt.Errorf("transaction size is 0")
		}

		bufTransaction := bytes.NewBuffer(blockBuffer.Next(transactionSize))
		t, err := transaction.UnmarshallTransaction(bufTransaction, fillData)
		if err != nil {
			if t != nil && t.TxHash() != nil {
				transaction.MarkTransactionBad(t.DbTransaction, t.TxHash(), err.Error())
			}
			return nil, fmt.Errorf("parse transaction error(%s)", err)
		}
		t.BlockData = &header

		transactions = append(transactions, t)

		// build merkle tree
		if len(t.FullData) > 0 {
			doubleHash := crypto.DoubleHash(t.FullData)
			doubleHash = converter.BinToHex(doubleHash)
			mrklSlice = append(mrklSlice, doubleHash)
		}
	}

	if len(mrklSlice) == 0 {
		mrklSlice = append(mrklSlice, []byte("0"))
	}
	mrkl, err := utils.MerkleTreeRoot(mrklSlice)
	if err != nil {
		return nil, err
	}
	return &Block{
		Header:            header,
		PrevRollbacksHash: prev.RollbacksHash,
		Transactions:      transactions,
		MrklRoot:          mrkl,
	}, nil
}
