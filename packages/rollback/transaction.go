/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package rollback

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

func rollbackUpdatedRow(tx map[string]string, where string, whereArgs []any, dbTx *sqldb.DbTransaction, logger *log.Entry) error {
	var rollbackInfo map[string]string
	if err := json.Unmarshal([]byte(tx["data"]), &rollbackInfo); err != nil {
		logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling rollback.Data from json")
		return err
	}
	addSQLUpdate, setArgs, err := rollbackAssignments(rollbackInfo, func(column string) bool {
		return syspar.IsByteColumn(tx["table_name"], column)
	})
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.ConversionError, "error": err, "rollback_id": tx["id"]}).Error("decoding bytea rollback data from hex")
		return err
	}
	if err := dbTx.Update(tx["table_name"], addSQLUpdate, where, append(setArgs, whereArgs...)...); err != nil {
		logger.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err, "rollback_id": tx["id"], "block_id": tx["block_id"], "where": where}).Error("updating table for rollback ")
		return err
	}
	return nil
}

// rollbackAssignments makes the SET list that puts back the columns of a row, in the order of the
// column names, and the values bound to its placeholders. The rollback data of a bytea column is
// the hex of its bytes; isByte is asked with the column name unquoted.
func rollbackAssignments(rollbackInfo map[string]string, isByte func(column string) bool) (string, []any, error) {
	keys := make([]string, 0, len(rollbackInfo))
	for k := range rollbackInfo {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	set := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for _, k := range keys {
		v := rollbackInfo[k]
		column := strings.Trim(k, `"`)
		k = `"` + column + `"`
		if v == "NULL" {
			set = append(set, k+`=NULL`)
		} else if isByte(column) && len(v) != 0 {
			if _, err := hex.DecodeString(v); err != nil {
				return "", nil, fmt.Errorf("column %s: %w", column, err)
			}
			set = append(set, k+`=decode(?,'hex')`)
			args = append(args, v)
		} else {
			set = append(set, k+`=?`)
			args = append(args, v)
		}
	}
	return strings.Join(set, ","), args, nil
}

func rollbackInsertedRow(tx map[string]string, where string, whereArgs []any, dbTx *sqldb.DbTransaction, logger *log.Entry) error {
	if err := dbTx.Delete(tx["table_name"], where, whereArgs...); err != nil {
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
			case "DeleteRow":
				err = smart.SysRollbackDeleteRow(dbTx, sysData)
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
		id := tx["table_id"]
		if len(tx["data"]) == 0 && isFirstTable {
			// the rollback of a row inserted into a table of the first ecosystem is "id,ecosystem"
			if a := strings.Split(id, ","); len(a) > 1 {
				id, ecoID = a[0], a[1]
			}
		}
		where, whereArgs := ` WHERE "id"=?`, []any{id}
		if isFirstTable {
			where += ` AND "ecosystem"=?`
			whereArgs = append(whereArgs, converter.Int64ToStr(converter.StrToInt64(ecoID)))
			tx[`table_name`] = `1_` + keyName
		}
		if len(tx["data"]) > 0 {
			if err := rollbackUpdatedRow(tx, where, whereArgs, dbTx, logger); err != nil {
				return err
			}
		} else {
			if err := rollbackInsertedRow(tx, where, whereArgs, dbTx, logger); err != nil {
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
