package transaction

import (
	"bytes"
	"time"

	"github.com/shopspring/decimal"

	"github.com/IBAX-io/go-ibax/packages/model"
)

func ProcessQueueTransactionBatches(dbTransaction *model.DbTransaction, qs []*model.QueueTx) error {
	var (
		checkTime = time.Now().Unix()
		hashes    model.ArrHashes
		trxs      []*model.Transaction
		hs        []byte
		err       error
	)

	defer func() {
		if err != nil {
			err = MarkTransactionBad(dbTransaction, hs, err.Error())
			if err != nil {
				return
			}
		}
	}()
	for i := 0; i < len(qs); i++ {
		binaryTx := qs[i].Data
		hs = qs[i].Hash
		tx := &Transaction{}
		}
		var expedite decimal.Decimal
		if len(tx.TxSmart.Expedite) > 0 {
			expedite, err = decimal.NewFromString(tx.TxSmart.Expedite)
			if err != nil {
				return err
			}
		}
		newTx := &model.Transaction{
			Hash:     hs,
			Data:     binaryTx,
			Type:     int8(tx.TxType),
			KeyID:    tx.TxKeyID,
			Expedite: expedite,
			Time:     tx.TxTime,
			Verified: 1,
			Used:     0,
			Sent:     0,
		}
		trxs = append(trxs, newTx)
		hashes = append(hashes, hs)
	}

	if len(trxs) > 0 {
		errTx := model.CreateTransactionBatches(dbTransaction, trxs)
		if errTx != nil {
			return errTx
		}
	}
	if len(hashes) > 0 {
		errQTx := model.DeleteQueueTxs(dbTransaction, hashes)
		if errQTx != nil {
			return errQTx
		}
	}
	return nil
}
