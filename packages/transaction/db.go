/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/utils"
)

var (
	ErrDuplicatedTx = errors.New("Duplicated transaction")
	ErrNotComeTime  = errors.New("Transaction processing time has not come")
	ErrExpiredTime  = errors.New("Transaction processing time is expired")
	ErrEarlyTime    = utils.WithBan(errors.New("Early transaction time"))
	ErrEmptyKey     = utils.WithBan(errors.New("KeyID is empty"))
)

// MarkTransactionBad takes a transaction found bad out of the pool, which answers its status
func MarkTransactionBad(hash []byte, errText string) {
	if hash == nil {
		return
	}
	if len(errText) > 255 {
		errText = errText[:255] + "..."
	}
	log.WithFields(log.Fields{"type": consts.BadTxError, "tx_hash": hash, "error": errText}).Debug("tx marked as bad")
	TxPool.Bad(hash, errText)
}
