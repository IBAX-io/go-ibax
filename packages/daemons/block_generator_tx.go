/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"encoding/hex"

	"github.com/IBAX-io/go-ibax/packages/script"

	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/IBAX-io/go-ibax/packages/transaction"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	log "github.com/sirupsen/logrus"
)

const (
	callDelayedContract = "CallDelayedContract"
	firstEcosystemID    = 1
)

// DelayedTx represents struct which works with delayed contracts
type DelayedTx struct {
	logger     *log.Entry
	privateKey string
	publicKey  string
	time       int64
}

// RunForDelayBlockID creates the transactions that need to be run for blockID
func (dtx *DelayedTx) RunForDelayBlockID(blockID int64) ([]*sqldb.Transaction, error) {

	contracts, err := sqldb.GetAllDelayedContractsForBlockID(blockID)
	if err != nil {
		dtx.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting delayed contracts for block")
		return nil, err
	}
	txList := make([]*sqldb.Transaction, 0, len(contracts))
	for _, c := range contracts {
		params := make(map[string]interface{})
		params["Id"] = c.ID
		tx, err := dtx.createDelayTx(c.KeyID, c.HighRate, params)
		if err != nil {
			dtx.logger.WithFields(log.Fields{"error": err}).Debug("can't create transaction for delayed contract")
			return nil, err
		}
		txList = append(txList, tx)
	}

	return txList, nil
}

func (dtx *DelayedTx) createDelayTx(keyID, highRate int64, params map[string]interface{}) (*sqldb.Transaction, error) {
	vm := script.GetVM()
	contract := smart.VMGetContract(vm, callDelayedContract, uint32(firstEcosystemID))
	info := contract.Info()

	smartTx := types.SmartContract{
		Header: &types.Header{
			ID:          int(info.ID),
			Time:        dtx.time,
			EcosystemID: firstEcosystemID,
			KeyID:       keyID,
			NetworkID:   conf.Config.LocalConf.NetworkID,
		},
		SignedBy: smart.PubToID(dtx.publicKey),
		Params:   params,
	}

	privateKey, err := hex.DecodeString(dtx.privateKey)
	if err != nil {
		return nil, err
	}

	txData, txHash, err := transaction.NewInternalTransaction(smartTx, privateKey)
	if err != nil {
		return nil, err
	}
	return transaction.CreateDelayTransactionHighRate(txData, txHash, keyID, highRate), nil
}
