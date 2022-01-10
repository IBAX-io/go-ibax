/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"strconv"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
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
	DbTransaction *sqldb.DbTransaction
	Data          types.FirstBlock
	TxHash        []byte
	Payload       []byte // transaction binary data
}

func (f *FirstBlockTransaction) txType() byte                { return types.FirstBlockTxType }
func (f *FirstBlockTransaction) txHash() []byte              { return f.TxHash }
func (f *FirstBlockTransaction) txPayload() []byte           { return f.Payload }
func (f *FirstBlockTransaction) txTime() int64               { return int64(f.Data.Time) }
func (f *FirstBlockTransaction) txKeyID() int64              { return f.Data.KeyID }
func (f *FirstBlockTransaction) txExpedite() decimal.Decimal { return decimal.Decimal{} }

func (f *FirstBlockTransaction) Init(*Transaction) error { return nil }

func (f *FirstBlockTransaction) Validate() error {
	return nil
}

func (f *FirstBlockTransaction) Action(t *Transaction) error {
	logger := f.Logger
	data := f.Data
	keyID := crypto.Address(data.PublicKey)
	nodeKeyID := crypto.Address(data.NodePublicKey)
	err := sqldb.ExecSchemaEcosystem(nil, firstEcosystemID, keyID, ``, keyID, firstAppID)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("executing ecosystem schema")
		return err
	}

	amount := decimal.New(consts.FounderAmount, int32(consts.MoneyDigits)).String()

	taxes := &sqldb.SystemParameter{Name: `taxes_wallet`}
	if err = taxes.SaveArray([][]string{{"1", converter.Int64ToStr(keyID)}}); err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("saving taxes_wallet array")
		return err
	}

	err = sqldb.GetDB(t.DbTransaction).Exec(`update "1_system_parameters" SET value = ? where name = 'test'`, strconv.FormatInt(data.Test, 10)).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating test parameter")
		return err
	}

	err = sqldb.GetDB(t.DbTransaction).Exec(`Update "1_system_parameters" SET value = ? where name = 'private_blockchain'`, strconv.FormatUint(data.PrivateBlockchain, 10)).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating private_blockchain")
		return err
	}

	if err = syspar.SysUpdate(t.DbTransaction); err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating syspar")
		return err
	}

	err = sqldb.GetDB(t.DbTransaction).Exec(`insert into "1_keys" (id,account,pub,amount) values(?,?,?,?),(?,?,?,?)`,
		keyID, converter.AddressToString(keyID), data.PublicKey, amount, nodeKeyID, converter.AddressToString(nodeKeyID), data.NodePublicKey, 0).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("inserting key")
		return err
	}
	id, err := t.DbTransaction.GetNextID("1_pages")
	if err != nil {
		return err
	}
	err = sqldb.GetDB(t.DbTransaction).Exec(`insert into "1_pages" (id,name,menu,value,conditions) values(?, 'default_page',
		  'default_menu', ?, 'ContractConditions("@1DeveloperCondition")')`,
		id, syspar.SysString(`default_ecosystem_page`)).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("inserting default page")
		return err
	}
	id, err = t.DbTransaction.GetNextID("1_menu")
	if err != nil {
		return err
	}
	err = sqldb.GetDB(t.DbTransaction).Exec(`insert into "1_menu" (id,name,value,title,conditions) values(?, 'default_menu', ?, ?, 'ContractAccess("@1EditMenu")')`,
		id, syspar.SysString(`default_ecosystem_menu`), `default`).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("inserting default menu")
		return err
	}
	err = smart.LoadContract(t.DbTransaction, 1)
	if err != nil {
		return err
	}
	if err := syspar.SysTableColType(t.DbTransaction); err != nil {
		return err
	}
	syspar.SetFirstBlockData(&data)
	return nil
}

func (f *FirstBlockTransaction) TxRollback() error {
	return nil
}

func (f *FirstBlockTransaction) Unmarshal(buffer *bytes.Buffer) error {
	buffer.UnreadByte()
	f.Payload = buffer.Bytes()
	f.TxHash = crypto.DoubleHash(f.Payload)
	if err := converter.BinUnmarshal(&f.Payload, &f.Data); err != nil {
		return err
	}
	return nil
}
