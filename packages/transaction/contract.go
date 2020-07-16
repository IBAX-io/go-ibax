/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/utils/tx"
)

const (
	errUnknownContract = `Cannot find %s contract`
)

func CreateContract(contractName string, keyID int64, params map[string]interface{},
	privateKey []byte) error {
			NetworkID:   conf.Config.NetworkID,
		},
		Params: params,
	}
	txData, _, err := tx.NewTransaction(sc, privateKey)
	if err == nil {
		rtx := &RawTransaction{}
		if err = rtx.Unmarshall(bytes.NewBuffer(txData)); err == nil {
			err = model.SendTx(rtx, sc.KeyID)
		}
	}
	return err
}
