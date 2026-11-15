/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/gorilla/mux"
)

func getTxRecord(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	hashes := params["hashes"]

	var (
		hashList   []string
		resultList []any
	)
	if len(hashes) > 0 {
		hashList = strings.Split(hashes, ",")
	}
	for _, hashStr := range hashList {

		blockID, txIdx, found, err := txPosition(converter.HexToBin(hashStr))
		if err != nil || !found {
			continue
		}
		if result, err := sqldb.GetTxRecord(nil, blockID, txIdx); err == nil {
			resultList = append(resultList, result)
		}
	}
	jsonResponse(w, &resultList)
}

// txPosition returns the block of the transaction hash and its index in the block
func txPosition(hash []byte) (int64, int, bool, error) {
	blockID, found, err := sqldb.TxBlock(hash)
	if err != nil || !found {
		return 0, 0, false, err
	}
	bk := &sqldb.BlockChain{}
	if found, err = bk.GetArchived(blockID); err != nil || !found {
		return 0, 0, false, err
	}
	b, err := block.UnmarshallBlock(bytes.NewBuffer(bk.Data), false)
	if err != nil {
		return 0, 0, false, err
	}
	for i, t := range b.Transactions {
		if bytes.Equal(t.Hash(), hash) {
			return blockID, i, true, nil
		}
	}
	return 0, 0, false, nil
}
