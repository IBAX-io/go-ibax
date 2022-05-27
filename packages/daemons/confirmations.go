/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

var tick int

// Confirmations gets and checks blocks from nodes
// Getting amount of nodes, which has the same hash as we do
func Confirmations(ctx context.Context, d *daemon) error {
	if atomic.CompareAndSwapUint32(&d.atomic, 0, 1) {
		defer atomic.StoreUint32(&d.atomic, 0)
	} else {
		return nil
	}
	// the first 2 minutes we sleep for 10 sec for blocks to be collected
	tick++

	d.sleepTime = 1 * time.Second
	if tick < 12 {
		d.sleepTime = 10 * time.Second
	}

	var startBlockID int64

	// check last blocks, but not more than 5
	confirmations := &sqldb.Confirmation{}
	_, err := confirmations.GetGoodBlock(consts.MinConfirmedNodes)
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting good block")
		return err
	}

	ConfirmedBlockID := confirmations.BlockID
	infoBlock := &sqldb.InfoBlock{}
	_, err = infoBlock.Get()
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting info block")
		return err
	}
	lastBlockID := infoBlock.BlockID
	if lastBlockID == 0 {
		return nil
	}

	if lastBlockID-ConfirmedBlockID > 5 {
		startBlockID = ConfirmedBlockID + 1
		d.sleepTime = 10 * time.Second
		tick = 0 // reset the tick
	}

	if startBlockID == 0 {
		startBlockID = lastBlockID
	}

	return confirmationsBlocks(ctx, d, lastBlockID, startBlockID)
}

func confirmationsBlocks(ctx context.Context, d *daemon, lastBlockID, startBlockID int64) error {
	for blockID := lastBlockID; blockID >= startBlockID; blockID-- {
		if err := ctx.Err(); err != nil {
			d.logger.WithFields(log.Fields{"type": consts.ContextError, "error": err}).Error("error in context")
			return err
		}

		block := sqldb.BlockChain{}
		_, err := block.Get(blockID)
		if err != nil {
			d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting block by ID")
			return err
		}

		hashStr := string(converter.BinToHex(block.Hash))
		d.logger.WithFields(log.Fields{"hash": hashStr}).Debug("checking hash")
		if len(hashStr) == 0 {
			d.logger.WithFields(log.Fields{"hash": hashStr, "type": consts.NotFound}).Debug("hash not found")
			continue
		}
		var hosts []string
		hosts, err = GetRemoteGoodHosts()
		if err != nil {
			return err
		}
		ch := make(chan string)
		for i := 0; i < len(hosts); i++ {
			host, err := tcpclient.NormalizeHostAddress(hosts[i], consts.DefaultTcpPort)
			if err != nil {
				d.logger.WithFields(log.Fields{"host": hosts[i], "type": consts.ParseError, "error": err}).Error("wrong host address")
				continue
			}

			d.logger.WithFields(log.Fields{"host": host, "block_id": blockID}).Debug("checking block id confirmed at node")
			go func() {
				IsReachable(host, blockID, ch, d.logger)
			}()
		}
		var answer string
		var st0, st1 int64
		for i := 0; i < len(hosts); i++ {
			answer = <-ch
			if answer == hashStr {
				st1++
			} else {
				st0++
			}
		}
		confirmation := &sqldb.Confirmation{}
		confirmation.GetConfirmation(blockID)
		confirmation.BlockID = blockID
		confirmation.Good = int32(st1)
		confirmation.Bad = int32(st0)
		confirmation.Time = time.Now().Unix()
		if err = confirmation.Save(); err != nil {
			d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("saving confirmation")
			return err
		}

		if blockID > startBlockID && st1 >= consts.MinConfirmedNodes {
			break
		}
	}

	return nil
}

// IsReachable checks if there is blockID on the host
func IsReachable(host string, blockID int64, ch0 chan string, logger *log.Entry) {
	ch := make(chan string, 1)
	go func() {
		ch <- tcpclient.CheckConfirmation(host, blockID, logger)
	}()
	select {
	case reachable := <-ch:
		ch0 <- reachable
	case <-time.After(consts.WaitConfirmedNodes * time.Second):
		ch0 <- "0"
	}
}
