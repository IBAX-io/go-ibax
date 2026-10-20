/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/language"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
)

// memState is the state outside the database that executing a block changes: the contracts of the
// virtual machine and the language sources. The platform parameters and the column types are
// reloaded from the committed database instead, and the parsed transactions are dropped.
type memState struct {
	vm   *script.VMSnapshot
	lang language.Snapshot
}

func takeMemState() memState {
	return memState{vm: script.GetVM().Snapshot(), lang: language.TakeSnapshot()}
}

func (m memState) restore() error {
	script.GetVM().Restore(m.vm)
	language.Restore(m.lang)
	transaction.CleanCache()
	if err := syspar.SysUpdate(nil); err != nil {
		return fmt.Errorf("reloading the platform parameters: %w", err)
	}
	if err := syspar.SysTableColType(nil); err != nil {
		return fmt.Errorf("reloading the column types: %w", err)
	}
	return nil
}

// Trial executes the block in a database transaction as its commit would, then rolls all of it
// back (F18 design 4.3): after it the database, the contracts of the virtual machine, the language
// sources and the platform parameters are as they were before. A validator prevotes a proposal only
// if its trial succeeds. The proposer makes its proposal by the trial of a generated block, which
// sets the transactions, the header and BinData of the block. Of a block that is not committed no
// notification is due and no node pauses; a validator neither bans a key nor marks a transaction
// bad for the block of another node, the proposer marks the transactions it leaves out as it does
// when it generates a block.
func (b *Block) Trial() error {
	mem := takeMemState()
	dbTx, err := sqldb.StartTransaction()
	if err != nil {
		return err
	}
	b.trial = true
	err = b.ProcessTxs(dbTx)
	if err == nil {
		if b.GenBlock && len(b.TxFullData) == 0 {
			err = ErrEmptyBlock
		} else {
			err = b.InsertIntoBlockchain(dbTx)
		}
	}
	b.trial = false
	b.Notifications = nil
	b.SysUpdate = false
	if errRoll := dbTx.Rollback(); errRoll != nil && err == nil {
		err = errRoll
	}
	if errMem := mem.restore(); errMem != nil {
		return errMem
	}
	return err
}

// Commit executes the block a certificate commits, as a block another node made, and stores the
// block and the certificate in one database transaction: a node has the certificate of a height
// if and only if it has its block. The block is parsed again from BinData, so nothing a trial left
// on the parsed transactions reaches the commit.
func Commit(binData []byte, cert *bft.Certificate) (*Block, error) {
	b, err := ProcessBlockByBinData(binData, true)
	if err != nil {
		return nil, err
	}
	if uint64(b.Header.BlockId) != cert.Height {
		return nil, fmt.Errorf("the certificate of height %d commits no block %d", cert.Height, b.Header.BlockId)
	}
	hash, err := bft.HashOf(b.Header.BlockHash)
	if err != nil {
		return nil, err
	}
	if hash != cert.Hash {
		return nil, fmt.Errorf("the certificate of height %d commits another block than %x", cert.Height, b.Header.BlockHash)
	}
	c := &sqldb.BftCertificate{
		Height: b.Header.BlockId,
		Round:  int64(cert.Round),
		Hash:   cert.Hash[:],
		Data:   cert.Marshal(),
	}
	if err := b.playSafe(c.Create); err != nil {
		return nil, err
	}
	return b, nil
}
