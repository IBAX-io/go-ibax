package sqldb

import (
	"gorm.io/gorm"

	"gorm.io/gorm/clause"

	"github.com/shopspring/decimal"
)

// SendTx is creates transaction
//func SendTx(rtx types.TransactionInfoer, adminWallet int64) error {
//	ts := &TransactionStatus{
//		Hash:     rtx.TxHashes(),
//		Time:     rtx.Time(),
//		Type:     rtx.Type(),
//		WalletID: adminWallet,
//	}
//	foundts, err := ts.Get(rtx.TxHashes())
//	if foundts {
//		log.WithFields(log.Fields{"tx_hash": rtx.TxHashes(), "wallet_id": adminWallet, "tx_time": ts.Time, "type": consts.DuplicateObject}).Error("double tx in transactions status")
//		return errors.New("duplicated transaction from transactions status")
//	}
//	if err != nil {
//		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting transaction from transactions status")
//		return err
//	}
//	err = ts.Create()
//	if err != nil {
//		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("transaction status create")
//		return err
//	}
//
//	qtx := &QueueTx{
//		Hash:     rtx.TxHashes(),
//		Data:     rtx.Bytes(),
//		Expedite: rtx.GetExpedite(),
//		Time:     rtx.Time(),
//	}
//	foundqx, err := qtx.GetByHash(nil, rtx.TxHashes())
//	if foundqx {
//		log.WithFields(log.Fields{"tx_hash": rtx.TxHashes(), "wallet_id": adminWallet, "tx_time": ts.Time, "type": consts.DuplicateObject}).Error("double tx in queue tx")
//		return errors.New("duplicated transaction from queue tx ")
//	}
//	if err != nil {
//		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting transaction from queue tx")
//		return err
//	}
//	return qtx.Create()
//}

type RawTx struct {
	TxType   byte
	Time     int64
	Hash     []byte
	Data     []byte
	Expedite string
	WalletID int64
}

func (rtx *RawTx) GetExpedite() decimal.Decimal {
	expedite, _ := decimal.NewFromString(rtx.Expedite)
	return expedite
}

func SendTxBatches(rtxs []*RawTx) error {
	var rawTxs []*TransactionStatus
	var qtxs []*QueueTx
	for _, rtx := range rtxs {
		ts := &TransactionStatus{
			Hash:     rtx.Hash,
			Time:     rtx.Time,
			Type:     rtx.TxType,
			WalletID: rtx.WalletID,
		}
		rawTxs = append(rawTxs, ts)
		qtx := &QueueTx{
			Hash:     rtx.Hash,
			Data:     rtx.Data,
			Expedite: rtx.GetExpedite(),
			Time:     rtx.Time,
		}
		qtxs = append(qtxs, qtx)
	}
	return DBConn.Clauses(clause.OnConflict{DoNothing: true}).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rawTxs).Error; err != nil {
			return err
		}
		if err := tx.Create(&qtxs).Error; err != nil {
			return err
		}
		return nil
	})

}
