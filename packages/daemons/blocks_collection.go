/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"
	"github.com/IBAX-io/go-ibax/packages/rollback"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/utils"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// BlocksCollection collects and parses blocks
func BlocksCollection(ctx context.Context, d *daemon) error {
	if ctx.Err() != nil {
		d.logger.WithFields(log.Fields{"type": consts.ContextError, "error": ctx.Err()}).Error("context error")
		return ctx.Err()
	}

	return blocksCollection(ctx, d)
}

func blocksCollection(ctx context.Context, d *daemon) (err error) {
	if !atomic.CompareAndSwapUint32(&d.atomic, 0, 1) {
		return nil
	}
	defer atomic.StoreUint32(&d.atomic, 0)

	//if !NtpDriftFlag {
	//	d.logger.WithFields(log.Fields{"type": consts.Ntpdate}).Error("ntp time not ntpdate")
	//	return nil
	//}

	host, maxBlockID, err := getHostWithMaxID(ctx, d.logger)
	if err != nil {
		d.logger.WithError(err).Warn("on checking best host")
		return err
	}

	infoBlock := &sqldb.InfoBlock{}
	found, err := infoBlock.Get()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting cur blockID")
		return err
	}
	if !found {
		log.WithFields(log.Fields{"type": consts.NotFound, "error": err}).Error("Info block not found")
		return errors.New("Info block not found")
	}

	if infoBlock.BlockID >= maxBlockID {
		log.WithFields(log.Fields{"blockID": infoBlock.BlockID, "maxBlockID": maxBlockID}).Debug("Max block is already in the host")
		return nil
	}

	DBLock()
	defer func() {
		node.NodeDoneUpdatingBlockchain()
		DBUnlock()
	}()

	// update our chain till maxBlockID from the host
	return UpdateChain(ctx, d, host, maxBlockID)
}

// UpdateChain load from host all blocks from our last block to maxBlockID
func UpdateChain(ctx context.Context, d *daemon, host string, maxBlockID int64) error {
	// get current block id from our blockchain
	curBlock := &sqldb.InfoBlock{}
	if _, err := curBlock.Get(); err != nil {
		d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting info block")
		return err
	}

	if ctx.Err() != nil {
		d.logger.WithFields(log.Fields{"type": consts.ContextError, "error": ctx.Err()}).Error("context error")
		return ctx.Err()
	}

	playRawBlock := func(rb []byte) error {
		var lastBlockID, lastBlockTime int64
		var err error
		var bl *block.Block
		defer func(err2 *error) {
			if err2 != nil {
				banNodePause(host, lastBlockID, lastBlockTime, *err2)
			}
		}(&err)
		bl, err = block.ProcessBlockByBinData(rb, true)
		if err != nil {
			d.logger.WithFields(log.Fields{"error": err, "type": consts.BlockError}).Error("processing block")
			return err
		}

		curBlock := &sqldb.InfoBlock{}
		if _, err = curBlock.Get(); err != nil {
			d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Getting info block")
			return err
		}

		if curBlock.BlockID != bl.PrevHeader.BlockId {
			d.logger.WithFields(log.Fields{"type": consts.BlockError}).Error("info block compare with previous block")
			return fmt.Errorf("info block compare with previous block err curBlock: %d, PrevBlock: %d", curBlock.BlockID, bl.PrevHeader.BlockId)
		}

		lastBlockID = bl.Header.BlockId
		lastBlockTime = bl.Header.Timestamp

		if err = bl.Check(); err != nil {
			var replaceCount int64 = 1
			if err == block.ErrIncorrectRollbackHash {
				replaceCount++
			}
			d.logger.WithFields(log.Fields{"error": err, "from_host": host,
				"different": fmt.Errorf("not match previous block %d, prev_position %d, current_position %d",
					bl.PrevHeader.BlockId,
					bl.PrevHeader.NodePosition,
					bl.Header.NodePosition),
				"type": consts.BlockError, "replaceCount": replaceCount}).Error("checking block hash")
			//if it is forked, replace the previous blocks to ones from the host
			if errReplace := ReplaceBlocksFromHost(ctx, host, bl.PrevHeader.BlockId, replaceCount); errReplace != nil {
				return errReplace
			}
			return err
		}
		return bl.PlaySafe()
	}

	var count int
	st := time.Now()

	d.logger.WithFields(log.Fields{"min_block": curBlock.BlockID, "max_block": maxBlockID, "count": maxBlockID - curBlock.BlockID}).Info("starting downloading blocks")
	for blockID := curBlock.BlockID + 1; blockID <= maxBlockID; blockID += int64(network.BlocksPerRequest) {
		if loopErr := func() error {
			ctxDone, cancel := context.WithCancel(ctx)
			defer func() {
				cancel()
				d.logger.WithFields(log.Fields{"count": count, "time": time.Since(st).String()}).Info("blocks downloaded")
			}()

			rawBlocksChan, err := tcpclient.GetBlocksBodies(ctxDone, host, blockID, false)
			if err != nil {
				d.logger.WithFields(log.Fields{"error": err, "type": consts.BlockError}).Error("getting block body")
				return err
			}

			for rawBlock := range rawBlocksChan {
				if err = playRawBlock(rawBlock); err != nil {
					// d.logger.WithFields(log.Fields{"error": err, "type": consts.BlockError}).Error("playing raw block")
					return err
				}
				count++
			}

			return nil
		}(); loopErr != nil {
			return loopErr
		}
	}
	return nil
}

func banNodePause(host string, blockID, blockTime int64, err error) {
	if err == nil || !utils.IsBanError(err) {
		return
	}

	reason := err.Error()
	//log.WithFields(log.Fields{"host": host, "block_id": blockID, "block_time": blockTime, "err": err}).Error("ban node")

	n, err := syspar.GetNodeByHost(host)
	if err != nil {
		log.WithError(err).Error("getting node by host")
		return
	}

	err = node.GetNodesBanService().RegisterBadBlock(n, blockID, blockTime, reason, false)
	if err != nil {
		log.WithFields(log.Fields{"error": err, "node": hex.EncodeToString(n.PublicKey),
			"block": blockID}).Error("registering bad block from node")
	}
}

// GetHostWithMaxID returns host with maxBlockID
func getHostWithMaxID(ctx context.Context, logger *log.Entry) (host string, maxBlockID int64, err error) {
	candidateNodes, err := sqldb.GetCandidateNode(syspar.SysInt(syspar.NumberNodes))
	if err == nil && len(candidateNodes) > 0 {
		syspar.SetRunModel(consts.CandidateNodeMode)
	} else {
		syspar.SetRunModel(consts.HonorNodeMode)
	}

	selectMode := SelectModel{}
	hosts, err := selectMode.GetHostWithMaxID()

	if err != nil {
		logger.WithError(err).Error("on filtering banned hosts")
	}

	host, maxBlockID, err = tcpclient.HostWithMaxBlock(ctx, hosts)
	if len(hosts) == 0 || err == tcpclient.ErrNodesUnavailable {
		hosts = conf.GetNodesAddr()
		return tcpclient.HostWithMaxBlock(ctx, hosts)
	}

	return
}

// ReplaceBlocksFromHost replaces blockchain received from the host.
// Number (replaceCount) of blocks starting from blockID will be re-played.
func ReplaceBlocksFromHost(ctx context.Context, host string, blockID, replaceCount int64) error {
	blocks, err := getBlocks(ctx, host, blockID, replaceCount)
	if err != nil {
		return err
	}
	transaction.CleanCache()

	// mark all transaction as unverified
	_, err = sqldb.MarkVerifiedAndNotUsedTransactionsUnverified()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"type":  consts.DBError,
		}).Error("marking verified and not used transactions unverified")
		return utils.ErrInfo(err)
	}

	// get starting blockID from slice of blocks
	if len(blocks) > 0 {
		blockID = blocks[len(blocks)-1].Header.BlockId
	}

	// we have the slice of blocks for applying
	// first of all we should rollback old blocks
	b := &sqldb.BlockChain{}
	myRollbackBlocks, err := b.GetBlocksFrom(blockID-1, "desc", 0)
	if err != nil {
		log.WithFields(log.Fields{"error": err, "type": consts.DBError}).Error("getting rollback blocks from blockID")
		return utils.ErrInfo(err)
	}
	for _, b := range myRollbackBlocks {
		err := rollback.RollbackBlock(b.Data)
		if err != nil {
			return utils.ErrInfo(err)
		}
	}

	script.SavepointSmartVMObjects()
	err = processBlocks(blocks)
	if err != nil {
		script.RollbackSmartVMObjects()
		return err
	}
	script.ReleaseSmartVMObjects()
	return err
}

func getBlocks(ctx context.Context, host string, blockID, minCount int64) ([]*block.Block, error) {
	rollback := syspar.GetRbBlocks1()
	blocks := make([]*block.Block, 0)
	nextBlockID := blockID

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// load the block bodies from the host
	blocksCh, err := tcpclient.GetBlocksBodies(ctx, host, blockID, true)
	if err != nil {
		return nil, utils.WithBan(errors.Wrapf(err, "Getting bodies of blocks by id %d", blockID))
	}

	for binaryBlock := range blocksCh {
		if blockID < 2 {
			break
		}

		// if the limit of blocks received from the node was exaggerated
		if len(blocks) >= int(rollback) {
			break
		}

		bl, err := block.ProcessBlockByBinData(binaryBlock, true)
		if err != nil {
			return nil, err
		}

		if bl.Header.BlockId != nextBlockID {
			log.WithFields(log.Fields{"header_block_id": bl.Header.BlockId, "block_id": blockID, "type": consts.InvalidObject}).Error("block ids does not match")
			return nil, utils.WithBan(errors.New("bad block_data['block_id']"))
		}

		// the public key of the one who has generated this block
		nodePublicKey, err := syspar.GetNodePublicKeyByPosition(bl.Header.NodePosition)
		if err != nil {
			log.WithFields(log.Fields{"header_block_id": bl.Header.BlockId, "block_id": blockID, "type": consts.InvalidObject}).Error("block ids does not match")
			return nil, utils.ErrInfo(err)
		}

		// save the block
		blocks = append(blocks, bl)

		// check the signature
		_, okSignErr := utils.CheckSign([][]byte{nodePublicKey},
			[]byte(bl.ForSign()),
			bl.Header.Sign, true)
		if okSignErr == nil && len(blocks) >= int(minCount) {
			break
		}

		nextBlockID--
	}

	return blocks, nil
}

func processBlocks(blocks []*block.Block) error {
	// go through new blocks from the smallest block_id to the largest block_id
	prevBlocks := make(map[int64]*block.Block, 0)
	for i := len(blocks) - 1; i >= 0; i-- {
		b := blocks[i]
		if _, ok := prevBlocks[b.Header.BlockId-1]; ok {
			b.PrevHeader = prevBlocks[b.Header.BlockId-1].Header
		}
		if err := b.Check(); err != nil {
			return err
		}
		if err := b.PlaySafe(); err != nil {
			return err
		}

		prevBlocks[b.Header.BlockId] = b

	}
	return nil
}
