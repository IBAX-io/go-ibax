/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package custom

import (
	"errors"
	"strconv"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/crypto"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/utils"
	"github.com/IBAX-io/go-ibax/packages/utils/tx"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

const (
	firstEcosystemID = 1
	firstAppID       = 1
)

// FirstBlockParser is parser wrapper
type FirstBlockTransaction struct {
	Logger        *log.Entry
	DbTransaction *model.DbTransaction
	Data          interface{}
}

// ErrFirstBlockHostIsEmpty host for first block is not specified
var ErrFirstBlockHostIsEmpty = errors.New("FirstBlockHost is empty")

// Init first block
func (t *FirstBlockTransaction) Init() error {
	return nil
}

// Validate first block
func (t *FirstBlockTransaction) Validate() error {
	return nil
}

// Action is fires first block
func (t *FirstBlockTransaction) Action() error {
	logger := t.Logger
	data := t.Data.(*consts.FirstBlock)
	keyID := crypto.Address(data.PublicKey)
	nodeKeyID := crypto.Address(data.NodePublicKey)
	err := model.ExecSchemaEcosystem(nil, firstEcosystemID, keyID, ``, keyID, firstAppID)
	err = model.GetDB(t.DbTransaction).Exec(`update "1_system_parameters" SET value = ? where name = 'test'`, strconv.FormatInt(data.Test, 10)).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating test parameter")
		return utils.ErrInfo(err)
	}

	err = model.GetDB(t.DbTransaction).Exec(`Update "1_system_parameters" SET value = ? where name = 'private_blockchain'`, strconv.FormatUint(data.PrivateBlockchain, 10)).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating private_blockchain")
		return utils.ErrInfo(err)
	}

	if err = syspar.SysUpdate(t.DbTransaction); err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating syspar")
		return utils.ErrInfo(err)
	}

	err = model.GetDB(t.DbTransaction).Exec(`insert into "1_keys" (id,account,pub,amount) values(?,?,?,?),(?,?,?,?)`,
		keyID, converter.AddressToString(keyID), data.PublicKey, amount, nodeKeyID, converter.AddressToString(nodeKeyID), data.NodePublicKey, 0).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("inserting key")
		return utils.ErrInfo(err)
	}
	id, err := model.GetNextID(t.DbTransaction, "1_pages")
	if err != nil {
		return utils.ErrInfo(err)
	}
	err = model.GetDB(t.DbTransaction).Exec(`insert into "1_pages" (id,name,menu,value,conditions) values(?, 'default_page',
		  'default_menu', ?, 'ContractConditions("@1DeveloperCondition")')`,
		id, syspar.SysString(`default_ecosystem_page`)).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("inserting default page")
		return utils.ErrInfo(err)
	}
	id, err = model.GetNextID(t.DbTransaction, "1_menu")
	if err != nil {
		return utils.ErrInfo(err)
	}
	err = model.GetDB(t.DbTransaction).Exec(`insert into "1_menu" (id,name,value,title,conditions) values(?, 'default_menu', ?, ?, 'ContractAccess("@1EditMenu")')`,
		id, syspar.SysString(`default_ecosystem_menu`), `default`).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("inserting default menu")
		return utils.ErrInfo(err)
	}
	err = smart.LoadContract(t.DbTransaction, 1)
	if err != nil {
		return utils.ErrInfo(err)
	}
	if err := syspar.SysTableColType(t.DbTransaction); err != nil {
		return utils.ErrInfo(err)
	}
	syspar.SetFirstBlockData(data)
	return nil
}

// Rollback first block
func (t *FirstBlockTransaction) Rollback() error {
	return nil
}

// Header is returns first block header
func (t FirstBlockTransaction) Header() *tx.Header {
	return nil
}
