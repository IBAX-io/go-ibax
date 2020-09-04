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
	contract := smart.GetContract(contractName, uint32(ecosysID))
	if contract == nil {
		return fmt.Errorf(errUnknownContract, contractName)
	}
	sc := tx.SmartContract{
		Header: tx.Header{
			ID:          int(contract.Block.Info.(*script.ContractInfo).ID),
			Time:        time.Now().Unix(),
			EcosystemID: ecosysID,
			KeyID:       keyID,
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
