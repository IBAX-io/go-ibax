package transaction

import (
	"encoding/hex"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
)

// GetLogger returns logger
func (t *Transaction) GetLogger() *log.Entry {
	var logger *log.Entry
	if t.Inner != nil {
		logger = log.WithFields(log.Fields{"tx_type": t.Type(), "tx_time": t.Timestamp(), "tx_wallet_id": t.KeyID()})
	}
	if t.BlockHeader != nil {
		logger = logger.WithFields(log.Fields{"block_id": t.BlockHeader.BlockId, "block_time": t.BlockHeader.Timestamp, "block_wallet_id": t.BlockHeader.KeyId, "block_state_id": t.BlockHeader.EcosystemId, "block_hash": t.BlockHeader.BlockHash, "block_version": t.BlockHeader.Version})
	}
	if t.PreBlockHeader != nil {
		logger = logger.WithFields(log.Fields{"pre_block_id": t.PreBlockHeader.BlockId, "pre_block_time": t.PreBlockHeader.Timestamp, "pre_block_wallet_id": t.PreBlockHeader.KeyId, "pre_block_state_id": t.PreBlockHeader.EcosystemId, "pre_block_hash": t.PreBlockHeader.BlockHash, "pre_block_version": t.PreBlockHeader.Version})
	}
	return logger
}

func (t *Transaction) Play() error {
	if err := t.Inner.Init(t.InToCxt); err != nil {
		return err
	}
	return t.Inner.Action(t.InToCxt, t.OutCtx)
}

func (t *Transaction) Check(checkTime int64) error {
	return CheckBatch([]*Transaction{t}, checkTime)[0]
}

// checkTime refuses a transaction of no key, or whose signed time is out of the window of
// checkTime: later than MaxTxForw after it, or, but for StopNetwork, earlier than MaxTxBack before
// it. The window is of the time the signature covers, not of the time a node received the
// transaction at, so that a transaction sent again past MaxTxBack is refused once its hash has
// left log_transactions (sqldb.Retain)
func (t *Transaction) checkTime(checkTime int64) error {
	if t.KeyID() == 0 {
		return ErrEmptyKey
	}
	signed := t.SignedTime()
	logger := log.WithFields(log.Fields{"tx_hash": hex.EncodeToString(t.Hash()), "tx_time": signed, "check_time": checkTime, "type": consts.ParameterExceeded})
	if signed > checkTime+consts.MaxTxForw {
		logger.Errorf("time in the tx cannot be more than %d seconds after the block time", consts.MaxTxForw)
		return ErrEarlyTime
	}
	if t.Type() != types.StopNetworkTxType {
		if signed < checkTime-consts.MaxTxBack {
			logger.WithFields(log.Fields{"tx_max_back": consts.MaxTxBack, "tx_type": t.Type()}).Errorf("time in the tx cannot be less then %d seconds of block time", consts.MaxTxBack)
			return ErrExpiredTime
		}
	}
	return nil
}

// CheckBatch checks each transaction as Check does, with one query of the transactions in blocks
func CheckBatch(txs []*Transaction, checkTime int64) []error {
	errs := make([]error, len(txs))
	hashes := make([][]byte, 0, len(txs))
	for i, t := range txs {
		if errs[i] = t.checkTime(checkTime); errs[i] == nil {
			hashes = append(hashes, t.Hash())
		}
	}
	known, err := sqldb.KnownLogTransactions(nil, hashes)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting log transactions by hash")
	}
	for i, t := range txs {
		switch {
		case errs[i] != nil:
		case err != nil:
			errs[i] = err
		case known[string(t.Hash())]:
			log.WithFields(log.Fields{"tx_hash": t.Hash(), "type": consts.DuplicateObject}).Warning("double tx in log transactions")
			errs[i] = ErrDuplicatedTx
		}
	}
	return errs
}

// CallContract calls the contract functions according to the specified flags
//func (t *Transaction) CallContract(point int) error {
//
//	var err error
//	t.TxSize = int64(len(t.Raw.payload))
//	t.VM = smart.GetVM()
//	t.ChildChain = false
//	t.Rollback = true
//	t.SysUpdate = false
//	t.RollBackTx = make([]*sqldb.RollbackTx, 0)
//	if t.GenBlock {
//		t.TimeLimit = syspar.GetMaxBlockGenerationTime()
//	}
//
//	t.TxResult, err = t.SmartContract.CallContract(point)
//	if err == nil && t.TxSmart != nil {
//		if t.Penalty {
//			if t.FlushRollback != nil {
//				flush := t.FlushRollback
//				for i := len(flush) - 1; i >= 0; i-- {
//					flush[i].FlushVM()
//				}
//			}
//		}
//		err = t.TxCheckLimits.CheckLimit(t)
//	}
//	if err != nil {
//		if t.FlushRollback != nil {
//			flush := t.FlushRollback
//			for i := len(flush) - 1; i >= 0; i-- {
//				flush[i].FlushVM()
//			}
//		}
//	}
//	return err
//}
/*
func (t *Transaction) CallChildChainContract() (resultContract string, flushRollback []smart.FlushInfo, err error) {

	t.TxSize = int64(len(t.Inner.TxPayload()))
	t.VM = smart.GetVM()
	t.ChildChain = true
	t.Rollback = false
	t.SysUpdate = false

	resultContract, err = t.SmartContract.CallContract(0)
	return
}
*/
