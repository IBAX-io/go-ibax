/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"sync/atomic"

	"github.com/IBAX-io/go-ibax/packages/transaction"

	//
	//infoBlock := &model.InfoBlock{}
	//_, err := infoBlock.Get()
	//if err != nil {
	//	d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting info block")
	//	return err
	//}
	//if infoBlock.BlockID == 0 {
	//	d.logger.Debug("no blocks for parsing")
	//	return nil
	//}

	p := new(transaction.Transaction)
	err := transaction.ProcessTransactionsQueue(p.DbTransaction)
	if err != nil {
		d.logger.WithFields(log.Fields{"error": err}).Error("parsing transactions")
		return err
	}
	//for {
	//	select {
	//	case attempt := <-transaction.ChanTxAttempt:
	//		if attempt {
	//			err = transaction.ProcessTransactionsAttempt(p.DbTransaction)
	//			if err != nil {
	//				d.logger.WithFields(log.Fields{"error": err}).Error("parsing transactions attempt")
	//				return err
	//			}
	//		}
	//	default:
	//		return nil
	//	}
	//}
	return nil
}
