/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"fmt"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

const (
	SysName = `@system`
)

type SysRollData struct {
	Type        string `json:"type,omitempty"`
	EcosystemID int64  `json:"ecosystem,omitempty"`
	ID          int64  `json:"id,omitempty"`
	Data        string `json:"data,omitempty"`
	TableName   string `json:"table,omitempty"`
}

func SysRollback(sc *SmartContract, data SysRollData) error {
	out, err := marshalJSON(data, `marshaling sys rollback`)
	if err != nil {
		return err
	}
	rollbackSys := &sqldb.RollbackTx{
		BlockID:   sc.BlockData.BlockID,
		TxHash:    sc.TxHash,
		NameTable: SysName,
		TableID:   converter.Int64ToStr(sc.TxSmart.EcosystemID),
		Data:      string(out),
	}
	sc.RollBackTx = append(sc.RollBackTx, rollbackSys)
	err = rollbackSys.Create(sc.DbTransaction)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("creating system  rollback")
		return err
	}
	return nil
}

// SysRollbackTable is rolling back table
func SysRollbackTable(DbTransaction *sqldb.DbTransaction, sysData SysRollData) error {
	err := sqldb.DropTable(DbTransaction, sysData.TableName)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("dropping table")
		return err
	}
	return nil
}

// SysRollbackView is rolling back table
func SysRollbackView(DbTransaction *sqldb.DbTransaction, sysData SysRollData) error {
	err := sqldb.DropView(DbTransaction, sysData.TableName)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("dropping view")
		return err
	}
	return nil
}

// SysRollbackColumn is rolling back column
func SysRollbackColumn(DbTransaction *sqldb.DbTransaction, sysData SysRollData) error {
	return sqldb.AlterTableDropColumn(DbTransaction, sysData.TableName, sysData.Data)
}

// SysRollbackContract performs rollback for the contract
func SysRollbackContract(name string, EcosystemID int64) error {
	vm := script.GetVM()
	if c := VMGetContract(vm, name, uint32(EcosystemID)); c != nil {
		id := c.Block.Info.(*script.ContractInfo).ID
		if int(id) != len(vm.Children)-1 {
			err := fmt.Errorf(eRollbackContract, id, len(vm.Children)-1)
			log.WithFields(log.Fields{"type": consts.VMError, "error": err}).Error("rollback contract")
			return err
		}
		vm.Children = vm.Children[:id]
		delete(vm.Objects, c.Name)
	}

	return nil
}

func SysRollbackNewContract(sysData SysRollData, EcosystemID string) error {
	contractList, err := script.ContractsList(sysData.Data)
	if err != nil {
		return err
	}
	for _, contract := range contractList {
		if err := SysRollbackContract(contract, converter.StrToInt64(EcosystemID)); err != nil {
			return err
		}
	}
	return nil
}

// SysFlushContract is flushing contract
func SysFlushContract(iroot interface{}, id int64, active bool) error {
	root := iroot.(*script.Block)
	if id != 0 {
		if len(root.Children) != 1 || root.Children[0].Type != script.ObjContract {
			return fmt.Errorf(`only one contract must be in the record`)
		}
	}
	for i, item := range root.Children {
		if item.Type == script.ObjContract {
			root.Children[i].Info.(*script.ContractInfo).Owner.TableID = id
			root.Children[i].Info.(*script.ContractInfo).Owner.Active = active
		}
	}
	script.VMFlushBlock(script.GetVM(), root)
	return nil
}

// SysSetContractWallet changes WalletID of the contract in smartVM
func SysSetContractWallet(tblid, state int64, wallet int64) error {
	for i, item := range script.GetVM().Block.Children {
		if item != nil && item.Type == script.ObjContract {
			cinfo := item.Info.(*script.ContractInfo)
			if cinfo.Owner.TableID == tblid && cinfo.Owner.StateID == uint32(state) {
				script.GetVM().Children[i].Info.(*script.ContractInfo).Owner.WalletID = wallet
			}
		}
	}
	return nil
}

// SysRollbackEditContract rollbacks the contract
func SysRollbackEditContract(transaction *sqldb.DbTransaction, sysData SysRollData,
	EcosystemID string) error {

	fields, err := sqldb.GetOneRowTransaction(transaction, `select * from "1_contracts" where id=?`,
		sysData.ID).String()
	if err != nil {
		return err
	}
	if len(fields["value"]) > 0 {
		var owner *script.OwnerInfo
		for i, item := range script.GetVM().Block.Children {
			if item != nil && item.Type == script.ObjContract {
				cinfo := item.Info.(*script.ContractInfo)
				if cinfo.Owner.TableID == sysData.ID &&
					cinfo.Owner.StateID == uint32(converter.StrToInt64(EcosystemID)) {
					owner = script.GetVM().Children[i].Info.(*script.ContractInfo).Owner
					break
				}
			}
		}
		if owner == nil {
			err = errContractNotFound
			log.WithFields(log.Fields{"type": consts.VMError, "error": err}).Error("getting existing contract")
			return err
		}
		wallet := owner.WalletID
		if len(fields["wallet_id"]) > 0 {
			wallet = converter.StrToInt64(fields["wallet_id"])
		}
		root, err := script.VMCompileBlock(script.GetVM(), fields["value"],
			&script.OwnerInfo{StateID: uint32(owner.StateID), WalletID: wallet, TokenID: owner.TokenID})
		if err != nil {
			log.WithFields(log.Fields{"type": consts.VMError, "error": err}).Error("compiling contract")
			return err
		}
		err = SysFlushContract(root, owner.TableID, owner.Active)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.VMError, "error": err}).Error("flushing contract")
			return err
		}
	} else if len(fields["wallet_id"]) > 0 {
		return SysSetContractWallet(sysData.ID, converter.StrToInt64(EcosystemID),
			converter.StrToInt64(fields["wallet_id"]))
	}
	return nil
}

// SysRollbackEcosystem is rolling back ecosystem
func SysRollbackEcosystem(DbTransaction *sqldb.DbTransaction, sysData SysRollData) error {
	tables := make([]string, 0)
	for table := range converter.FirstEcosystemTables {
		tables = append(tables, table)
		err := sqldb.Delete(DbTransaction, `1_`+table, fmt.Sprintf(`where ecosystem='%d'`, sysData.ID))
		if err != nil {
			return err
		}
	}
	if sysData.ID == 1 {
		tables = append(tables, `node_ban_logs`, `bad_blocks`, `system_parameters`, `ecosystems`)
		for _, name := range tables {
			err := sqldb.DropTable(DbTransaction, fmt.Sprintf("%d_%s", sysData.ID, name))
			if err != nil {
				log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("dropping table")
				return err
			}
		}
	} else {
		vm := script.GetVM()
		for vm.Children[len(vm.Children)-1].Type == script.ObjContract {
			cinfo := vm.Children[len(vm.Children)-1].Info.(*script.ContractInfo)
			if int64(cinfo.Owner.StateID) != sysData.ID {
				break
			}
			if err := SysRollbackContract(cinfo.Name, sysData.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// SysRollbackActivate sets Deactive status of the contract in smartVM
func SysRollbackActivate(sysData SysRollData) error {
	ActivateContract(sysData.ID, sysData.EcosystemID, false)
	return nil
}

// SysRollbackDeactivate sets Active status of the contract in smartVM
func SysRollbackDeactivate(sysData SysRollData) error {
	ActivateContract(sysData.ID, sysData.EcosystemID, true)
	return nil
}

// SysRollbackDeleteColumn is rolling back delete column
func SysRollbackDeleteColumn(DbTransaction *sqldb.DbTransaction, sysData SysRollData) error {
	var (
		data map[string]string
	)
	err := unmarshalJSON([]byte(sysData.Data), &data, `rollback delete to json`)
	if err != nil {
		return err
	}
	sqlColType, err := columnType(data["type"])
	if err != nil {
		return err
	}
	err = sqldb.AlterTableAddColumn(DbTransaction, sysData.TableName, data["name"], sqlColType)
	if err != nil {
		return logErrorDB(err, "adding column to the table")
	}
	return nil
}

// SysRollbackDeleteTable is rolling back delete table
func SysRollbackDeleteTable(DbTransaction *sqldb.DbTransaction, sysData SysRollData) error {
	var (
		data    TableInfo
		colsSQL string
	)
	err := unmarshalJSON([]byte(sysData.Data), &data, `rollback delete table to json`)
	if err != nil {
		return err
	}
	for key, item := range data.Columns {
		colsSQL += `"` + key + `" ` + typeToPSQL[item] + " ,\n"
	}
	err = sqldb.CreateTable(DbTransaction, sysData.TableName, strings.TrimRight(colsSQL, ",\n"))
	if err != nil {
		return logErrorDB(err, "creating tables")
	}

	prefix, _ := PrefixName(sysData.TableName)
	data.Table.SetTablePrefix(prefix)
	err = data.Table.Create(DbTransaction)
	if err != nil {
		return logErrorDB(err, "insert table info")
	}
	return nil
}
