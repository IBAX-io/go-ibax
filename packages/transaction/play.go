package transaction

import (
	"encoding/hex"
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"
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
	if t.KeyID() == 0 {
		return ErrEmptyKey
	}
	logger := log.WithFields(log.Fields{"tx_hash": hex.EncodeToString(t.Hash()), "tx_time": t.Timestamp(), "check_time": checkTime, "type": consts.ParameterExceeded})
	if time.UnixMilli(t.Timestamp()).Unix() > checkTime {
		//if time.UnixMilli(t.Timestamp()).Unix()-consts.MaxTxForw > checkTime {
		//	logger.WithFields(log.Fields{"tx_max_forw": consts.MaxTxForw}).Errorf("time in the tx cannot be more than %d seconds of block time ", consts.MaxTxForw)
		//	return ErrNotComeTime
		//}
		logger.Error("time in the tx cannot be more than of block time ")
		return ErrEarlyTime
	}

	if t.Type() != types.StopNetworkTxType {
		if time.UnixMilli(t.Timestamp()).Unix() < checkTime-consts.MaxTxBack {
			logger.WithFields(log.Fields{"tx_max_back": consts.MaxTxBack, "tx_type": t.Type()}).Errorf("time in the tx cannot be less then %d seconds of block time", consts.MaxTxBack)
			return ErrExpiredTime
		}
	}
	err := CheckLogTx(t.Hash(), logger)
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
