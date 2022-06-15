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

type DeliveProvider interface {
	SysUpdateWorker(*sqldb.DbTransaction) error
	SysTableColByteaWorker(*sqldb.DbTransaction) error
	FlushVM()
}

type InToCxt struct {
	SqlDbSavePoint int
	GenBlock       bool
	DbTransaction  *sqldb.DbTransaction
	BlockHeader    *types.BlockHeader
	PreBlockHeader *types.BlockHeader
	Notifications  types.Notifications
	Rand           *rand.Rand
	TxCheckLimits  *Limits
}

type OutCtx struct {
	SysUpdate  bool
	RollBackTx []*types.RollbackTx
	TxResult   *pbgo.TxResult
}
