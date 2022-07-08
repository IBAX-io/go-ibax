package transaction

import (
	"bytes"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/protocols"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
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
		tx, err = UnmarshallTransaction(bytes.NewBuffer(qs[i].Data))
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

func ProcessTransactions(logger *log.Entry, txs []*sqldb.Transaction, st time.Time) ([][]byte, error) {
	var done = make(<-chan time.Time, 1)
	if syspar.IsHonorNodeMode() {
		btc := protocols.NewBlockTimeCounter()
		_, endTime, err := btc.RangeByTime(st)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.TimeCalcError, "error": err}).Error("on getting end time of generation")
			return nil, err
		}
		done = time.After(endTime.Sub(st))
	}
	trs, err := sqldb.GetAllUnusedTransactions(nil, syspar.GetMaxTxCount())
	if err != nil {
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting all unused transactions")
		return nil, err
	}

	limits := NewLimits(GetLetPreprocess())

	type badTxStruct struct {
		hash  []byte
		msg   string
		keyID int64
	}

	processBadTx := func() chan badTxStruct {
		ch := make(chan badTxStruct)
		go func() {
			for badTxItem := range ch {
				BadTxForBan(badTxItem.keyID)
				_ = MarkTransactionBad(badTxItem.hash, badTxItem.msg)
			}
		}()
		return ch
	}
	txBadChan := processBadTx()
	defer func() {
		close(txBadChan)
	}()

	// Checks preprocessing count limits
	txList := make([][]byte, 0, len(trs))
	txs = append(txs, trs...)
	for i, txItem := range txs {
		if syspar.IsHonorNodeMode() {
			select {
			case <-done:
				return txList, nil
			default:
			}
		}
		if txItem.GetTransactionRateStopNetwork() {
			txList = append(txList[:0], txs[i].Data)
			break
		}
		bufTransaction := bytes.NewBuffer(txItem.Data)
		tr, err := UnmarshallTransaction(bufTransaction)
		if err != nil {
			if tr != nil {
				txBadChan <- badTxStruct{hash: tr.Hash(), msg: err.Error(), keyID: tr.KeyID()}
			}
			continue
		}

		if err := tr.Check(st.Unix()); err != nil {
			txBadChan <- badTxStruct{hash: tr.Hash(), msg: err.Error(), keyID: tr.KeyID()}
			continue
		}

		if tr.IsSmartContract() {
			err = limits.CheckLimit(tr.Inner)
			if errors.Cause(err) == ErrLimitStop && i > 0 {
				break
			} else if err != nil {
				if err != ErrLimitSkip {
					txBadChan <- badTxStruct{hash: tr.Hash(), msg: err.Error(), keyID: tr.KeyID()}
				}
				continue
			}
		}
		txList = append(txList, txs[i].Data)
	}
	return txList, nil
}
