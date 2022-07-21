/*----------------------------------------------------------------
- Copyright (c) IBAX. All rights reserved.
- See LICENSE in the project root for license information.
---------------------------------------------------------------*/

package block

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/common/random"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/notificator"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// PlaySafe is inserting block safely
func (b *Block) PlaySafe() error {
	logger := b.GetLogger()
	dbTx, err := sqldb.StartTransaction()
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("starting db transaction")
		return err
	}

	inputTx := b.Transactions[:]
	err = b.ProcessTxs(dbTx)
	if err != nil {
		dbTx.Rollback()
		if b.GenBlock && len(b.TxFullData) == 0 {
			var banK = make(map[string]struct{}, len(inputTx))
			for i := 0; i < len(inputTx); i++ {
				var t, k = inputTx[i], strconv.FormatInt(inputTx[i].KeyID(), 10)
				if t.IsSmartContract() {
					if _, ok := banK[k]; !ok {
						transaction.BadTxForBan(t.KeyID())
						banK[k] = struct{}{}
						continue
					}
				}
				if err := transaction.MarkTransactionBad(t.Hash(), err.Error()); err != nil {
					return err
				}
			}
		}
		return err
	}

	if b.GenBlock && len(b.TxFullData) == 0 {
		dbTx.Commit()
		return ErrEmptyBlock
	}

	if err := b.InsertIntoBlockchain(dbTx); err != nil {
		dbTx.Rollback()
		return err
	}
	err = dbTx.Commit()
	if err != nil {
		return err
	}
	for _, q := range b.Notifications {
		q.Send()
	}
	return nil
}

func (b *Block) ProcessTxs(dbTx *sqldb.DbTransaction) (err error) {
	afters := &types.AfterTxs{
		Rts: make([]*types.RollbackTx, 0),
		Txs: make([]*types.AfterTx, 0),
	}
	logger := b.GetLogger()
	limits := transaction.NewLimits(b.limitMode())
	rand := random.NewRand(b.Header.Timestamp)
	processedTx := make([][]byte, 0, len(b.Transactions))
	var genBErr error
	defer func() {
		if b.IsGenesis() || b.GenBlock {
			b.AfterTxs = afters
		}
		if b.GenBlock {
			b.TxFullData = processedTx
		}
		if genBErr != nil {
			err = genBErr
		}
		if errA := b.AfterPlayTxs(dbTx); errA != nil {
			if err == nil {
				err = errA
			} else if err != nil {
				err = fmt.Errorf("%v; %w", err, errA)
			}
			return
		}
	}()
	if !b.GenBlock && !b.IsGenesis() && conf.Config.BlockSyncMethod.Method == types.BlockSyncMethod_SQLDML.String() {
		if b.SysUpdate {
			if err := syspar.SysUpdate(dbTx); err != nil {
				return fmt.Errorf("updating syspar: %w", err)
			}
		}
		return nil
	}
	for curTx := 0; curTx < len(b.Transactions); curTx++ {
		t := b.Transactions[curTx]
		err := dbTx.Savepoint(consts.SetSavePointMarkBlock(curTx))
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err, "tx_hash": t.Hash()}).Error("using savepoint")
			return err
		}
		err = t.WithOption(notificator.NewQueue(), b.GenBlock, b.Header, b.PrevHeader, dbTx, rand.BytesSeed(t.Hash()), limits, curTx)
		if err != nil {
			return err
		}
		err = t.Play()
		if err != nil {
			if err == transaction.ErrNetworkStopping {
				// Set the node in a pause state
				node.PauseNodeActivity(node.PauseTypeStopingNetwork)
				return err
			}
			errRoll := t.DbTransaction.RollbackSavepoint(consts.SetSavePointMarkBlock(t.InToCxt.SqlDbSavePoint))
			if errRoll != nil {
				return fmt.Errorf("%v; %w", err, errRoll)
			}
			if b.GenBlock {
				if errors.Cause(err) == transaction.ErrLimitStop {
					if curTx == 0 {
						return err
					}
					break
				}
				if strings.Contains(err.Error(), script.ErrVMTimeLimit.Error()) {
					err = script.ErrVMTimeLimit
				}
			}
			if t.IsSmartContract() {
				transaction.BadTxForBan(t.KeyID())
			}
			_ = transaction.MarkTransactionBad(t.Hash(), err.Error())
			if t.SysUpdate {
				if err := syspar.SysUpdate(t.DbTransaction); err != nil {
					return fmt.Errorf("updating syspar: %w", err)
				}
				t.SysUpdate = false
			}
			if b.GenBlock {
				genBErr = err
				continue
			}
			return err
		}

		if t.SysUpdate {
			b.SysUpdate = true
			t.SysUpdate = false
		}

		if t.Notifications.Size() > 0 {
			b.Notifications = append(b.Notifications, t.Notifications)
		}

		var (
			after    = &types.AfterTx{}
			eco      int64
			contract string
			code     pbgo.TxInvokeStatusCode
		)
		if t.IsSmartContract() {
			eco = t.SmartContract().TxSmart.EcosystemID
			code = t.TxResult.Code
			if t.SmartContract().TxContract != nil {
				contract = t.SmartContract().TxContract.Name
			}
		}
		after.UsedTx = t.Hash()
		after.Lts = &types.LogTransaction{
			Block:        t.BlockHeader.BlockId,
			Hash:         t.Hash(),
			TxData:       t.FullData,
			Timestamp:    t.Timestamp(),
			Address:      t.KeyID(),
			EcosystemId:  eco,
			ContractName: contract,
			InvokeStatus: code,
		}
		after.UpdTxStatus = t.TxResult
		afters.Txs = append(afters.Txs, after)
		afters.Rts = append(afters.Rts, t.RollBackTx...)
		afters.TxBinLogSql = append(afters.TxBinLogSql, t.DbTransaction.BinLogSql...)
		processedTx = append(processedTx, t.FullData)
	}
	return nil
}
