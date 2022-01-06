/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

const (
	errUnknownContract = `Cannot find %s contract`
)

func CreateContract(contractName string, keyID int64, params map[string]interface{},
	privateKey []byte) error {
	ecosysID, _ := converter.ParseName(contractName)
	if ecosysID == 0 {
		ecosysID = 1
	}
	contract := smart.GetContract(contractName, uint32(ecosysID))
	if contract == nil {
		return fmt.Errorf(errUnknownContract, contractName)
	}
	sc := types.SmartContract{
		Header: &types.Header{
			ID:          int(contract.Info().ID),
			Time:        time.Now().Unix(),
			EcosystemID: ecosysID,
			KeyID:       keyID,
			NetworkID:   conf.Config.LocalConf.NetworkID,
		},
		Params: params,
	}
	txData, _, err := NewTransaction(sc, privateKey)
	if err == nil {
		rtx := &Transaction{}
		if err = rtx.Unmarshall(bytes.NewBuffer(txData)); err == nil {
			//err = sqldb.SendTx(rtx, sc.KeyID)
			err = sqldb.SendTxBatches([]*sqldb.RawTx{rtx.SetRawTx()})
		}
	}
	return err
}
