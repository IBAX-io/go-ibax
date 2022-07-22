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
		tx, err := dtx.createDelayTxByItem(c.Contract, c.KeyID, c.HighRate)
		if err != nil {
			dtx.logger.WithError(err).Debug("can't create transaction for delayed contract")
			return nil, err
		}
		txList = append(txList, tx)
	}

	return txList, nil
}

func (dtx *DelayedTx) createDelayTxByItem(name string, keyID, highRate int64) (*sqldb.Transaction, error) {
	vm := script.GetVM()
	contract := smart.VMGetContract(vm, name, uint32(firstEcosystemID))
	smartTx := types.SmartTransaction{
		Header: &types.Header{
			ID:          int(contract.Info().ID),
			EcosystemID: firstEcosystemID,
			KeyID:       keyID,
			Time:        dtx.time,
			NetworkID:   conf.Config.LocalConf.NetworkID,
		},
		SignedBy: smart.PubToID(dtx.publicKey),
		Params:   map[string]any{},
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
