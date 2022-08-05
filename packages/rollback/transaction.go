/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package rollback

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

func rollbackUpdatedRow(tx map[string]string, where string, dbTx *sqldb.DbTransaction, logger *log.Entry) error {
	var rollbackInfo map[string]string
	if err := json.Unmarshal([]byte(tx["data"]), &rollbackInfo); err != nil {
		logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling rollback.Data from json")
		return err
	}
	addSQLUpdate := ""
	for k, v := range rollbackInfo {
		k = `"` + strings.Trim(k, `"`) + `"`
		if v == "NULL" {
			addSQLUpdate += k + `=NULL,`
		} else if syspar.IsByteColumn(tx["table_name"], k) && len(v) != 0 {
			addSQLUpdate += k + `=decode('` + v + `','HEX'),`
		} else {
			addSQLUpdate += k + `='` + strings.Replace(v, `'`, `''`, -1) + `',`
		}
	}
	addSQLUpdate = addSQLUpdate[0 : len(addSQLUpdate)-1]
	if err := dbTx.Update(tx["table_name"], addSQLUpdate, where); err != nil {
		logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err, "rollback_id": tx["id"], "block_id": tx["block_id"], "update": addSQLUpdate, "where": where}).Error("updating table for rollback ")
		return err
	}
	return nil
}

func rollbackInsertedRow(tx map[string]string, where string, dbTx *sqldb.DbTransaction, logger *log.Entry) error {
	if err := dbTx.Delete(tx["table_name"], where); err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "rollback_id": tx["id"], "table": tx["table_name"], "where": where}).Error("deleting from table for rollback")
		return err
	}
	return nil
}

func rollbackTransaction(txHash []byte, dbTx *sqldb.DbTransaction, logger *log.Entry) error {
	rollbackTx := &sqldb.RollbackTx{}
	txs, err := rollbackTx.GetRollbackTransactions(dbTx, txHash)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting rollback transactions")
		return err
	}
	for _, tx := range txs {
		if tx["table_name"] == smart.SysName {
			var sysData smart.SysRollData
			err := json.Unmarshal([]byte(tx["data"]), &sysData)
			if err != nil {
				logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling rollback.Data from json")
				return err
			}
			switch sysData.Type {
			case "NewTable":
				err = smart.SysRollbackTable(dbTx, sysData)
			case "NewView":
				err = smart.SysRollbackView(dbTx, sysData)
			case "NewColumn":
				err = smart.SysRollbackColumn(dbTx, sysData)
			case "NewContract":
				err = smart.SysRollbackNewContract(sysData, tx["table_id"])
			case "EditContract":
				err = smart.SysRollbackEditContract(dbTx, sysData, tx["table_id"])
			case "NewEcosystem":
				err = smart.SysRollbackEcosystem(dbTx, sysData)
			case "ActivateContract":
				err = smart.SysRollbackActivate(sysData)
			case "DeactivateContract":
				err = smart.SysRollbackDeactivate(sysData)
			case "DeleteColumn":
				err = smart.SysRollbackDeleteColumn(dbTx, sysData)
			case "DeleteTable":
				err = smart.SysRollbackDeleteTable(dbTx, sysData)
			}
			if err != nil {
				return err
			}
			continue
		}
		table := tx[`table_name`]
		var (
			rollbackInfo   map[string]string
			ecoID, keyName string
			isFirstTable   bool
		)
		if len(tx["data"]) > 0 {
			if err := json.Unmarshal([]byte(tx["data"]), &rollbackInfo); err != nil {
				logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling rollback.Data from json")
				return err
			}
			if len(rollbackInfo) > 0 {
				if v, ok := rollbackInfo["ecosystem"]; ok {
					ecoID = v
				}
			}
		}
		if under := strings.IndexByte(table, '_'); under > 0 {
			keyName = table[under+1:]
			if v, ok := converter.FirstEcosystemTables[keyName]; ok && v {
				isFirstTable = true
			}
		}
		where := ` WHERE "id"='`

		if len(tx["data"]) <= 0 {
			if isFirstTable {
				var a []string
				if strings.Contains(tx["table_id"], ",") {
					a = strings.Split(tx["table_id"], ",")
					ecoID = a[1]
					where += a[0] + `'`
				}
			}
		}
		if len(tx["data"]) > 0 && isFirstTable {
			where += tx["table_id"] + `'`
		}
		if isFirstTable {
			where += fmt.Sprintf(` AND "ecosystem"='%d'`, converter.StrToInt64(ecoID))
			tx[`table_name`] = `1_` + keyName
		} else {
			where += tx["table_id"] + `'`
		}
		if len(tx["data"]) > 0 {
			if err := rollbackUpdatedRow(tx, where, dbTx, logger); err != nil {
				return err
			}
		} else {
			if err := rollbackInsertedRow(tx, where, dbTx, logger); err != nil {
				return err
			}
		}
	}
	txForDelete := &sqldb.RollbackTx{TxHash: txHash}
	err = txForDelete.DeleteByHash(dbTx)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting rollback transaction by hash")
		return err
	}
	return nil
}
