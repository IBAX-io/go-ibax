package sqldb

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/migration"
	"github.com/IBAX-io/go-ibax/packages/migration/childchain"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

// ExecSchemaEcosystem is executing ecosystem schema
func ExecSchemaEcosystem(db *DbTransaction, data migration.SqlData) error {
	if data.Ecosystem == 1 {
		q, err := migration.GetCommonEcosystemScript()
		if err != nil {
			return err
		}
		if err := db.ExecSql(q); err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("executing comma ecosystem schema")
			return err
		}
	}
	q, err := migration.GetEcosystemScript(data)
	if err != nil {
		return err
	}
	if err := db.ExecSql(q); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("executing ecosystem schema")
		return err
	}
	if data.Ecosystem == 1 {
		q, err = migration.GetFirstEcosystemScript(data)
		if err != nil {
			return err
		}
		if err := db.ExecSql(q); err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("executing first ecosystem schema")
			return err
		}
	}
	// Every ecosystem declares the shared tables it keeps rows in, as the first one does: without them the
	// node would take its pages, menus and other rows for tables of its own and refuse to read them
	q, err = migration.GetTableScript(data)
	if err != nil {
		return err
	}
	if err := db.ExecSql(q); err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("executing tables schema")
		return err
	}
	return nil
}

func ExecSubSchema() error {
	if conf.Config.IsSubNode() {
		if err := migration.InitMigrate(&MigrationHistory{}); err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on executing clb script")
			return err
		}
	}
	return nil
}

// ExecChildChainSchema is executing schema for off blockchainService
func ExecChildChainSchema(id int, wallet int64) error {

	if conf.Config.IsSupportingChildChain() {
		if err := migration.InitMigrate(&MigrationHistory{}); err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on executing clb script")
			return err
		}

		query := fmt.Sprintf(childchain.GetChildChainScript(), id, wallet, converter.AddressToString(wallet))
		if err := DBConn.Exec(query).Error; err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on executing clb script")
			return err
		}

		keyFile := func(name string) ([]byte, error) {
			data, err := os.ReadFile(filepath.Join(conf.Config.DirPathConf.KeysDir, name))
			if err != nil {
				log.WithFields(log.Fields{"type": consts.IOError, "error": err}).Error("reading key from file")
				return nil, err
			}
			return hex.DecodeString(strings.TrimSpace(string(data)))
		}
		nodePriv, err := keyFile(consts.NodePrivateKeyFilename)
		if err != nil {
			return err
		}
		nodePub, err := crypto.NodePrivateToPublic(nodePriv)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("converting node private key to public")
			return err
		}
		nodeKey := crypto.AccountKey{Algo: crypto.NodeAlgo(), Raw: nodePub}
		pub, err := keyFile(consts.PublicKeyFilename)
		if err != nil {
			return err
		}
		accountKey, err := crypto.ParseAccountKey(pub)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("reading the account key")
			return err
		}
		// The child chain accepts the algorithms of its two keys
		algos := fmt.Sprintf(`[{"algo":%q}]`, nodeKey.Algo)
		if accountKey.Algo != nodeKey.Algo {
			algos = fmt.Sprintf(`[{"algo":%q},{"algo":%q}]`, nodeKey.Algo, accountKey.Algo)
		}
		if err = GetDB(nil).Exec(`update "1_platform_parameters" set value = ? where name = 'account_algorithms'`, algos).Error; err != nil {
			return err
		}
		nodeKeyID, keyID := nodeKey.Address(), accountKey.Address()
		amount := decimal.New(consts.FounderAmount, int32(consts.MoneyDigits)).String()
		if err = GetDB(nil).Exec(`insert into "1_keys" (id,account,pub,amount) values (?,?,?,?),(?,?,?,?)`,
			keyID, converter.AddressToString(keyID), accountKey.Bytes(), amount, nodeKeyID, converter.AddressToString(nodeKeyID), nodeKey.Bytes(), 0).Error; err != nil {
			return err
		}
	}
	return nil
}

// ExecSchema is executing schema
func ExecSchema() error {
	return migration.InitMigrate(&MigrationHistory{})
}

// UpdateSchema run update migrations
func UpdateSchema() error {
	if !conf.Config.IsChainHost() {
		b := &BlockChain{}
		if found, err := b.GetMaxBlock(); !found {
			return err
		}
	}
	return migration.UpdateMigrate(&MigrationHistory{})
}
