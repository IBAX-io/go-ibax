package transaction

import (
	"encoding/hex"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
)

// GetLogger returns logger
func (t *Transaction) GetLogger() *log.Entry {
	var logger *log.Entry
	if t.Inner != nil {
		logger = log.WithFields(log.Fields{"tx_type": t.TxType(), "tx_time": t.TxTime(), "tx_wallet_id": t.TxKeyID()})
	}
	if t.BlockData != nil {
		logger = logger.WithFields(log.Fields{"block_id": t.BlockData.BlockID, "block_time": t.BlockData.Time, "block_wallet_id": t.BlockData.KeyID, "block_state_id": t.BlockData.EcosystemID, "block_hash": t.BlockData.Hash, "block_version": t.BlockData.Version})
	}
	if t.PreBlockData != nil {
		logger = logger.WithFields(log.Fields{"block_id": t.BlockData.BlockID, "block_time": t.BlockData.Time, "block_wallet_id": t.BlockData.KeyID, "block_state_id": t.BlockData.EcosystemID, "block_hash": t.BlockData.Hash, "block_version": t.BlockData.Version})
	}
	return logger
}

func (t *Transaction) Play() (err error) {
	err = t.Inner.Init(t)
	if err != nil {
		return
	}

	return t.Inner.Action(t)
}

func (t *Transaction) Check(checkTime int64) error {
	if t.TxKeyID() == 0 {
		return ErrEmptyKey
	}

	logger := log.WithFields(log.Fields{"tx_hash": hex.EncodeToString(t.TxHash()), "tx_time": t.TxTime(), "check_time": checkTime, "type": consts.ParameterExceeded})
	if t.TxTime() > checkTime {
		if t.TxTime()-consts.MaxTxForw > checkTime {
			logger.WithFields(log.Fields{"tx_max_forw": consts.MaxTxForw}).Errorf("time in the tx cannot be more than %d seconds of block time ", consts.MaxTxForw)
			return ErrNotComeTime
		}
		logger.Error("time in the tx cannot be more than of block time ")
		return ErrEarlyTime
	}

	if t.TxType() != types.StopNetworkTxType {
		if t.TxTime() < checkTime-consts.MaxTxBack {
			logger.WithFields(log.Fields{"tx_max_back": consts.MaxTxBack, "tx_type": t.TxType()}).Errorf("time in the tx cannot be less then %d seconds of block time", consts.MaxTxBack)
			return ErrExpiredTime
		}
	}
	err := CheckLogTx(t.TxHash(), logger)
	if err != nil {
		return err
	}

	return nil
}

// CallContract calls the contract functions according to the specified flags
//func (t *Transaction) CallContract(point int) error {
//
//	var err error
//	t.TxSize = int64(len(t.Raw.payload))
//	t.VM = smart.GetVM()
//	t.CLB = false
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
func (t *Transaction) CallCLBContract() (resultContract string, flushRollback []smart.FlushInfo, err error) {

	t.TxSize = int64(len(t.Inner.TxPayload()))
	t.VM = smart.GetVM()
	t.CLB = true
	t.Rollback = false
	t.SysUpdate = false

	resultContract, err = t.SmartContract.CallContract(0)
	return
}
*/
