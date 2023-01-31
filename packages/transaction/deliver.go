/*----------------------------------------------------------------
- Copyright (c) IBAX. All rights reserved.
- See LICENSE in the project root for license information.
---------------------------------------------------------------*/

package transaction

import (
	"math/rand"

	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
)

type DeliverProvider interface {
	SysUpdateWorker(*sqldb.DbTransaction) error
	SysTableColByteaWorker(*sqldb.DbTransaction) error
	FlushVM()
}

type InToCxt struct {
	SqlDbSavePoint string
	GenBlock       bool
	DbTransaction  *sqldb.DbTransaction
	BlockHeader    *types.BlockHeader
	PreBlockHeader *types.BlockHeader
	Notifications  types.Notifications
	Rand           *rand.Rand
	TxCheckLimits  *Limits
	OutputsMap     map[sqldb.KeyUTXO][]sqldb.SpentInfo
	PrevSysPar     map[string]string
	ComPercents    map[int64]int64
}

type OutCtx struct {
	SysUpdate    bool
	RollBackTx   []*types.RollbackTx
	TxResult     *pbgo.TxResult
	TxOutputsMap map[sqldb.KeyUTXO][]sqldb.SpentInfo
	TxInputsMap  map[sqldb.KeyUTXO][]sqldb.SpentInfo
}

type OutCtxOption func(b *OutCtx)

func (tr *OutCtx) Apply(opts ...OutCtxOption) {
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(tr)
	}
	return
}

func WithOutCtxTxResult(ret *pbgo.TxResult) OutCtxOption {
	return func(b *OutCtx) {
		b.TxResult = ret
	}
}

func WithOutCtxSysUpdate(ret bool) OutCtxOption {
	return func(b *OutCtx) {
		b.SysUpdate = ret
	}
}

func WithOutCtxRollBackTx(ret []*types.RollbackTx) OutCtxOption {
	return func(b *OutCtx) {
		b.RollBackTx = ret
	}
}

func WithOutCtxTxOutputs(txOutputsMap map[sqldb.KeyUTXO][]sqldb.SpentInfo) OutCtxOption {
	return func(b *OutCtx) {
		b.TxOutputsMap = txOutputsMap
	}
}

func WithOutCtxTxInputs(txInputsMap map[sqldb.KeyUTXO][]sqldb.SpentInfo) OutCtxOption {
	return func(b *OutCtx) {
		b.TxInputsMap = txInputsMap
	}
}
