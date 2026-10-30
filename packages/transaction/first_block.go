/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package transaction

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/migration"

	"github.com/pkg/errors"

	"github.com/vmihailenco/msgpack/v5"

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
type FirstBlockParser struct {
	storedSize
	Logger        *log.Entry
	DbTransaction *sqldb.DbTransaction
	Data          *types.FirstBlock
	Timestamp     int64
	TxHash        []byte
	Payload       []byte // transaction binary data
}

func (f *FirstBlockParser) txType() byte                { return f.Data.TxType() }
func (f *FirstBlockParser) txHash() []byte              { return f.TxHash }
func (f *FirstBlockParser) txPayload() []byte           { return f.Payload }
func (f *FirstBlockParser) txTime() int64               { return f.Timestamp }
func (f *FirstBlockParser) txSignedTime() int64         { return f.Data.Time }
func (f *FirstBlockParser) txKeyID() int64              { return f.Data.KeyID }
func (f *FirstBlockParser) txExpedite() decimal.Decimal { return decimal.Decimal{} }
func (s *FirstBlockParser) setTimestamp()               { s.Timestamp = time.Now().UnixMilli() }

func (f *FirstBlockParser) TxRollback() error                                      { return nil }
func (f *FirstBlockParser) SysUpdateWorker(dbTx *sqldb.DbTransaction) error        { return nil }
func (f *FirstBlockParser) SysTableColByteaWorker(dbTx *sqldb.DbTransaction) error { return nil }
func (f *FirstBlockParser) FlushVM()                                               {}

func (f *FirstBlockParser) Init(in *InToCxt) error {
	f.Logger = log.WithFields(log.Fields{})
	f.DbTransaction = in.DbTransaction
	return nil
}

func (f *FirstBlockParser) Validate() error {
	return nil
}

func (f *FirstBlockParser) Action(in *InToCxt, out *OutCtx) (err error) {
	if in.BlockHeader.BlockId > 1 {
		return nil
	}
	logger := f.Logger
	data := f.Data
	dbTx := in.DbTransaction
	algos, founder, err := CheckFirstBlockAccounts(data)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("checking the account keys of the first block")
		return err
	}
	nodeKey := crypto.NodeAccountKey(data.NodePublicKey)
	keyID := founder.Address()
	nodeKeyID := nodeKey.Address()
	err = sqldb.ExecSchemaEcosystem(dbTx, migration.SqlData{
		Ecosystem:   firstEcosystemID,
		Wallet:      keyID,
		Name:        consts.DefaultEcosystemName,
		Founder:     keyID,
		AppID:       firstAppID,
		Account:     converter.AddressToString(keyID),
		Digits:      consts.MoneyDigits,
		TokenSymbol: consts.DefaultTokenSymbol,
		TokenName:   consts.DefaultTokenName,
	})
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("executing ecosystem schema")
		return err
	}

	_, allocs, err := FirstBlockSupply(data)
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.InvalidObject, "error": err}).Error("checking the supply of the first block")
		return err
	}

	taxes := &sqldb.PlatformParameter{Name: `taxes_wallet`}
	if err = taxes.SaveArray(dbTx, [][]string{{"1", converter.Int64ToStr(keyID)}}); err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("saving taxes_wallet array")
		return err
	}

	err = sqldb.GetDB(dbTx).Exec(`update "1_platform_parameters" SET value = ? where name = 'test'`, strconv.FormatInt(data.Test, 10)).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating test parameter")
		return err
	}

	err = sqldb.GetDB(dbTx).Exec(`Update "1_platform_parameters" SET value = ? where name = 'private_blockchain'`, strconv.FormatUint(data.PrivateBlockchain, 10)).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating private_blockchain")
		return err
	}

	err = sqldb.GetDB(dbTx).Exec(`update "1_platform_parameters" SET value = ? where name = ?`, strconv.FormatUint(data.HonorNodesOnly, 10), syspar.HonorNodesOnly).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating honor_nodes_only")
		return err
	}

	err = sqldb.GetDB(dbTx).Exec(`update "1_platform_parameters" SET value = ? where name = ?`, strconv.FormatUint(data.BftConsensus, 10), syspar.BftConsensus).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating bft_consensus")
		return err
	}

	if data.BftConsensus == 1 {
		err = sqldb.GetDB(dbTx).Exec(`update "1_platform_parameters" SET value = ? where name = ?`, data.HonorNodes, syspar.HonorNodes).Error
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating honor_nodes")
			return err
		}
		for _, p := range syspar.BftParameters {
			err = sqldb.GetDB(dbTx).Exec(`insert into "1_platform_parameters" (id, name, value, conditions)
				values (next_id('1_platform_parameters'), ?, ?, 'ContractAccess("@1UpdatePlatformParam")')`, p[0], p[1]).Error
			if err != nil {
				logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "name": p[0]}).Error("inserting a platform parameter of the BFT consensus")
				return err
			}
		}
	}

	err = sqldb.GetDB(dbTx).Exec(`update "1_platform_parameters" SET value = ? where name = ?`, algos.String(), syspar.AccountAlgorithms).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating account_algorithms")
		return err
	}

	if err = syspar.SysUpdate(dbTx); err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("updating syspar")
		return err
	}

	err = sqldb.GetDB(dbTx).Exec(`insert into "1_keys" (id,account,pub,amount) values(?,?,?,?),(?,?,?,?)`,
		keyID, converter.AddressToString(keyID), founder.Bytes(), 0, nodeKeyID, converter.AddressToString(nodeKeyID), nodeKey.Bytes(), 0).Error
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("inserting key")
		return err
	}
	created := map[int64]bool{keyID: true, nodeKeyID: true}
	insert := func(id int64, pub []byte) error {
		if created[id] {
			return nil
		}
		created[id] = true
		err := sqldb.GetDB(dbTx).Exec(`insert into "1_keys" (id,account,pub) values(?,?,?)`, id, converter.AddressToString(id), pub).Error
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "key_id": id}).Error("inserting an account of the first block")
		}
		return err
	}

	// Each validator has the account of its node, which signs the delayed contracts of the blocks
	// it proposes, and the account of its operator, which takes their fees, as the node of the
	// first block has its own and the founder's
	if data.BftConsensus == 1 {
		operators, err := BftOperators(data.HonorNodes)
		if err != nil {
			return err
		}
		var nodes []*syspar.HonorNode
		if err := json.Unmarshal([]byte(data.HonorNodes), &nodes); err != nil {
			return err
		}
		for i, n := range nodes {
			key := crypto.NodeAccountKey(n.PublicKey)
			if err := insert(key.Address(), key.Bytes()); err != nil {
				return err
			}
			if err := insert(operators[i], []byte{}); err != nil {
				return err
			}
		}
	}

	// Each allocation credits its account: the founder's, or one the first block creates without
	// a key, of the conditions of the allocation, a part of the supply that only these contracts
	// move (CallContractAs), as CheckTxSign refuses every transaction signed for it. The coins are
	// an output of the first block, which TransferSelf moves to the account; a chain of the honor
	// nodes only takes no TransferSelf, so there they are the balance of the account.
	outputs := map[sqldb.KeyUTXO][]sqldb.SpentInfo{}
	for i, a := range allocs {
		if a.KeyID != keyID {
			if created[a.KeyID] {
				return fmt.Errorf("allocation %s: account %s has a key", a.Name, converter.AddressToString(a.KeyID))
			}
			created[a.KeyID] = true
			err := sqldb.GetDB(dbTx).Exec(`insert into "1_keys" (id,account,pub,conditions) values(?,?,?,?)`,
				a.KeyID, converter.AddressToString(a.KeyID), []byte{}, a.Conditions).Error
			if err != nil {
				logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "allocation": a.Name}).Error("inserting the account of an allocation")
				return err
			}
		}
		if data.HonorNodesOnly != 1 {
			// the block writes the outputs with the outputs of its transactions
			outputs[sqldb.KeyUTXO{Ecosystem: 1, KeyId: a.KeyID}] = []sqldb.SpentInfo{{OutputIndex: int32(i),
				OutputKeyId: a.KeyID, OutputValue: a.Amount, Ecosystem: 1, BlockId: 1, Type: consts.UTXO_Type_First_Block}}
			continue
		}
		err := sqldb.GetDB(dbTx).Exec(`update "1_keys" set amount = ? where id = ? and ecosystem = 1`, a.Amount, a.KeyID).Error
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "allocation": a.Name}).Error("crediting an allocation of the first block")
			return err
		}
	}
	if len(outputs) > 0 {
		out.Apply(WithOutCtxTxOutputs(outputs))
	}

	err = smart.LoadContract(dbTx, 1)
	if err != nil {
		return err
	}
	if err := syspar.SysTableColType(dbTx); err != nil {
		return err
	}
	syspar.SetFirstBlockData(data)
	syspar.SetFirstBlockTimestamp(time.UnixMilli(f.Timestamp).Unix())
	return nil
}

// CheckFirstBlockAccounts checks the accounts of the first block: account_algorithms is a valid
// set the node can verify; the node algorithm is in it, since the node account signs with the
// node key; the founder key is an account key that may be registered at genesis; and the supply
// and the allocations are those of FirstBlockSupply.
func CheckFirstBlockAccounts(data *types.FirstBlock) (syspar.AccountAlgorithmSet, crypto.AccountKey, error) {
	algos, err := syspar.ParseAccountAlgorithms(data.AccountAlgorithms)
	if err != nil {
		return nil, crypto.AccountKey{}, err
	}
	if err := algos.CheckNode(); err != nil {
		return nil, crypto.AccountKey{}, err
	}
	if err := algos.CheckRegister(crypto.NodeAlgo(), data.Time); err != nil {
		return nil, crypto.AccountKey{}, fmt.Errorf("the node account: %w", err)
	}
	if _, err := crypto.NewAccountKey(crypto.NodeAlgo(), data.NodePublicKey); err != nil {
		return nil, crypto.AccountKey{}, fmt.Errorf("the node key: %w", err)
	}
	founder, err := crypto.ParseAccountKey(data.PublicKey)
	if err != nil {
		return nil, crypto.AccountKey{}, fmt.Errorf("the founder key: %w", err)
	}
	if err := algos.CheckRegister(founder.Algo, data.Time); err != nil {
		return nil, crypto.AccountKey{}, fmt.Errorf("the founder key: %w", err)
	}
	if founder.Address() != data.KeyID {
		return nil, crypto.AccountKey{}, fmt.Errorf("the founder key is not that of key %d", data.KeyID)
	}
	if err := CheckFirstBlockConsensus(data); err != nil {
		return nil, crypto.AccountKey{}, err
	}
	if _, _, err := FirstBlockSupply(data); err != nil {
		return nil, crypto.AccountKey{}, err
	}
	return algos, founder, nil
}

// CheckFirstBlockConsensus checks the consensus of the first block. A chain of the BFT consensus
// (F18) is a chain of the honor nodes only, as the candidate node mode would change who generates
// the blocks by votes of the candidates, and its first block names all its validators, at least
// syspar.MinBftValidators of them, the node that generates it first. Another chain names none:
// its nodes join after the first block, as before.
func CheckFirstBlockConsensus(data *types.FirstBlock) error {
	switch data.BftConsensus {
	case 0:
		if len(data.HonorNodes) > 0 {
			return errors.New("the first block names the honor nodes only on a chain of the BFT consensus")
		}
		return nil
	case 1:
	default:
		return fmt.Errorf("bft_consensus is %d: it is 0 or 1", data.BftConsensus)
	}
	if data.HonorNodesOnly != 1 {
		return errors.New("a chain of the BFT consensus is a chain of the honor nodes only")
	}
	var nodes []*syspar.HonorNode
	if err := json.Unmarshal([]byte(data.HonorNodes), &nodes); err != nil {
		return fmt.Errorf("the honor nodes of the first block: %w", err)
	}
	if err := syspar.CheckBftValidators(nodes); err != nil {
		return fmt.Errorf("the honor nodes of the first block: %w", err)
	}
	if !bytes.Equal(nodes[0].PublicKey, data.NodePublicKey) || nodes[0].Stopped {
		return errors.New("the first honor node of the first block is the node that generates it")
	}
	operators, err := BftOperators(data.HonorNodes)
	if err != nil {
		return fmt.Errorf("the honor nodes of the first block: %w", err)
	}
	if operators[0] != data.KeyID {
		return fmt.Errorf("the first honor node of the first block is operated by key %d, the founder of the chain", data.KeyID)
	}
	return nil
}

// BftOperators is the key_id of each node of honor_nodes, in their order: the account of the
// operator of the node, which the fees of the blocks it proposes go to (its KeyID) and
// NodeOwnerCondition reads. On a chain of the BFT consensus the first block names it for each
// validator and creates its account of ecosystem 1, with the account of its node, since no
// transaction may create an account there and a proposer without them has every transaction and
// every delayed contract of its block fail.
func BftOperators(honorNodes string) ([]int64, error) {
	var nodes []struct {
		KeyID json.RawMessage `json:"key_id"`
	}
	if err := json.Unmarshal([]byte(honorNodes), &nodes); err != nil {
		return nil, err
	}
	ids := make([]int64, len(nodes))
	for i, n := range nodes {
		id, err := strconv.ParseInt(strings.Trim(string(n.KeyID), `"`), 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("honor node %d has no key_id of its operator", i)
		}
		ids[i] = id
	}
	return ids, nil
}

func (s *FirstBlockParser) BinMarshal(data *types.FirstBlock) ([]byte, error) {
	s.Data = data
	var buf []byte
	var err error
	buf, err = msgpack.Marshal(data)
	if err != nil {
		return nil, err
	}
	s.setTimestamp()
	s.Payload = buf
	s.TxHash = crypto.DoubleHash(s.Payload)
	buf, err = msgpack.Marshal(s)
	if err != nil {
		return nil, err
	}
	buf = append([]byte{s.txType()}, buf...)
	return buf, nil
}

func (f *FirstBlockParser) Unmarshal(buffer *bytes.Buffer) error {
	buffer.UnreadByte()
	if err := msgpack.Unmarshal(buffer.Bytes()[1:], f); err != nil {
		return errors.Wrap(err, "first block Unmarshal err")
	}
	return nil
}
