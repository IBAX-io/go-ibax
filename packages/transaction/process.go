package transaction

import (
	"bytes"
	"time"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

func ProcessQueueTransactionBatches(dbTx *sqldb.DbTransaction, qs []*sqldb.QueueTx) error {
	var (
		checkTime = time.Now().Unix()
		hashes    [][]byte
		trxs      []*sqldb.Transaction
		err       error
	)
	type badTxStruct struct {
		hash  []byte
		msg   string
		keyID int64
	}

	processBadTx := func(dbTx *sqldb.DbTransaction) chan badTxStruct {
		ch := make(chan badTxStruct)

		go func() {
			for badTxItem := range ch {
				BadTxForBan(badTxItem.keyID)
				_ = MarkTransactionBad(badTxItem.hash, badTxItem.msg)
			}
		}()

		return ch
	}

	txBadChan := processBadTx(dbTx)

	defer func() {
		close(txBadChan)
	}()

	for i := 0; i < len(qs); i++ {
		tx := &Transaction{}
		tx, err = UnmarshallTransaction(bytes.NewBuffer(qs[i].Data), true)
		if err != nil {
			if tx != nil {
				txBadChan <- badTxStruct{hash: tx.Hash(), msg: err.Error(), keyID: tx.KeyID()}
			}
			continue
		}
		err = tx.Check(checkTime)
		if err != nil {
			txBadChan <- badTxStruct{hash: tx.Hash(), msg: err.Error(), keyID: tx.KeyID()}
			continue
		}
		newTx := &sqldb.Transaction{
			Hash:     tx.Hash(),
			Data:     tx.FullData,
			Type:     int8(tx.Type()),
			KeyID:    tx.KeyID(),
			Expedite: tx.Expedite(),
			Time:     tx.Timestamp(),
			Verified: 1,
			Used:     0,
			Sent:     0,
		}
		trxs = append(trxs, newTx)
		hashes = append(hashes, qs[i].Hash)
	}

	if len(trxs) > 0 {
		errTx := sqldb.CreateTransactionBatches(dbTx, trxs)
		if errTx != nil {
			return errTx
		}
	}
	if len(hashes) > 0 {
		errQTx := sqldb.DeleteQueueTxs(dbTx, hashes)
		if errQTx != nil {
			return errQTx
		}
	}
	return nil
}
