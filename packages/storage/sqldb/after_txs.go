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
		Lts:         make([]*LogTransaction, len(after.Lts)),
		UpdTxStatus: make([]*UpdateBlockMsg, len(after.UpdTxStatus)),
	}
	playTx.UsedTx = after.UsedTx
	for i := 0; i < len(after.Rts); i++ {
		rt := new(RollbackTx)
		rt.BlockID = after.Rts[i].BlockId
		rt.NameTable = after.Rts[i].NameTable
		rt.Data = after.Rts[i].Data
		rt.TableID = after.Rts[i].TableId
		rt.TxHash = after.Rts[i].TxHash
		playTx.Rts[i] = rt
	}
	for i := 0; i < len(after.Lts); i++ {
		lt := new(LogTransaction)
		lt.Block = after.Lts[i].Block
		lt.Hash = after.Lts[i].Hash
		lt.TxData = after.Lts[i].TxData
		lt.Timestamp = after.Lts[i].Timestamp
		lt.Address = after.Lts[i].Address
		lt.EcosystemID = after.Lts[i].EcosystemId
		lt.ContractName = after.Lts[i].ContractName
		playTx.Lts[i] = lt
	}
	for i := 0; i < len(after.UpdTxStatus); i++ {
		u := new(UpdateBlockMsg)
		u.Hash = after.UpdTxStatus[i].Hash
		u.Msg = after.UpdTxStatus[i].Msg
		playTx.UpdTxStatus[i] = u
	}
	return playTx
}

func AfterPlayTxs(dbTx *DbTransaction, blockID int64, after *types.AfterTxs, genBlock, firstBlock bool) error {
	playTx := GenAfterTxs(after)
	return GetDB(dbTx).Transaction(func(tx *gorm.DB) error {
		if !genBlock && !firstBlock {
			//for i := 0; i < len(playTx.TxExecutionSql); i++ {
			//	if err := tx.Exec(string(playTx.TxExecutionSql[i])).Error; err != nil {
			//		return errors.Wrap(err, "batches exec sql for tx")
			//	}
			//}
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
