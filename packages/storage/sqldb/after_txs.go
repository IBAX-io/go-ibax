package sqldb

import (
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type AfterTxs struct {
	UsedTx      [][]byte
	Rts         []*RollbackTx
	Lts         []*LogTransaction
	UpdTxStatus []*UpdateBlockMsg
}

func GenAfterTxs(after *types.AfterTxs) *AfterTxs {
	playTx := &AfterTxs{
		Rts:         make([]*RollbackTx, len(after.Rts)),
		Lts:         make([]*LogTransaction, len(after.Txs)),
		UpdTxStatus: make([]*UpdateBlockMsg, len(after.Txs)),
	}

	for i := 0; i < len(after.Rts); i++ {
		tx := after.Rts[i]
		rt := new(RollbackTx)
		rt.BlockID = tx.BlockId
		rt.NameTable = tx.NameTable
		rt.Data = tx.Data
		rt.TableID = tx.TableId
		rt.TxHash = tx.TxHash
		playTx.Rts[i] = rt
	}

	for i := 0; i < len(after.Txs); i++ {
		tx := after.Txs[i]
		playTx.UsedTx = append(playTx.UsedTx, tx.UsedTx)
		lt := new(LogTransaction)
		lt.Block = tx.Lts.Block
		lt.Hash = tx.Lts.Hash
		lt.TxData = tx.Lts.TxData
		lt.Timestamp = tx.Lts.Timestamp
		lt.Address = tx.Lts.Address
		lt.EcosystemID = tx.Lts.EcosystemId
		lt.ContractName = tx.Lts.ContractName
		playTx.Lts[i] = lt

		u := new(UpdateBlockMsg)
		u.Hash = tx.UpdTxStatus.Hash
		u.Msg = tx.UpdTxStatus.Msg
		playTx.UpdTxStatus[i] = u
	}
	return playTx
}

func AfterPlayTxs(dbTx *DbTransaction, blockID int64, after *types.AfterTxs, genBlock, firstBlock bool) error {
	playTx := GenAfterTxs(after)
	return GetDB(dbTx).Transaction(func(tx *gorm.DB) error {
		if !genBlock && !firstBlock {
			for i := 0; i < len(after.TxExecutionSql); i++ {
				if err := tx.Exec(string(after.TxExecutionSql[i])).Error; err != nil {
					return errors.Wrap(err, "batches exec sql for tx")
				}
			}
		}
		if err := DeleteTransactions(tx, playTx.UsedTx); err != nil {
			return errors.Wrap(err, "batches delete used transactions")
		}
		if err := CreateLogTransactionBatches(tx, playTx.Lts); err != nil {
			return errors.Wrap(err, "batches insert log_transactions")
		}
		if err := CreateBatchesRollbackTx(tx, playTx.Rts); err != nil {
			return errors.Wrap(err, "batches insert rollback tx")
		}
		if err := UpdateBlockMsgBatches(tx, blockID, playTx.UpdTxStatus); err != nil {
			return errors.Wrap(err, "batches update block msg transaction status")
		}

		return nil
	})
}
