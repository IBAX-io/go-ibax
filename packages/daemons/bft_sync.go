/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"
	"github.com/IBAX-io/go-ibax/packages/service/node"

	log "github.com/sirupsen/logrus"
)

// bftFetchTimeout is the timeout of a request of certificates
const bftFetchTimeout = 10 * time.Second

// updateChainBft is the synchronization of a chain of the BFT consensus (design 4.2): a node
// behind takes the blocks after its last block from the node with the most blocks, each with its
// certificate, and commits a block only if its certificate verifies by the validators set of its
// height. A committed block is final, so there is no fork to replace, and a block that fails bans
// no node: the node takes it again, from this node or another one.
func updateChainBft(ctx context.Context, d *daemon) error {
	hosts := syspar.GetRemoteHosts()
	host, maxBlockID, err := tcpclient.HostWithMaxBlock(ctx, hosts)
	if len(hosts) == 0 || err == tcpclient.ErrNodesUnavailable {
		host, maxBlockID, err = tcpclient.HostWithMaxBlock(ctx, conf.GetNodesAddr())
	}
	if err != nil {
		d.logger.WithError(err).Warn("on checking best host")
		return err
	}
	last, err := lastBlock()
	if err != nil {
		return err
	}
	if maxBlockID <= int64(last) {
		return nil
	}

	lockWrites()
	defer func() {
		node.NodeDoneUpdatingBlockchain()
		DBUnlock()
	}()
	if last, err = lastBlock(); err != nil {
		return err
	}
	// the driver may have committed the blocks while the node waited for the lock
	if maxBlockID <= int64(last) {
		return nil
	}
	d.logger.WithFields(log.Fields{"from": last + 1, "to": maxBlockID, "host": host}).Info("synchronizing by certificates")
	for h := int64(last) + 1; h <= maxBlockID; {
		n, err := syncCertified(ctx, host, h, maxBlockID)
		if errors.Is(err, tcpclient.ErrNotKept) {
			// the blocks are out of the window of the host: a node that keeps them goes on
			var m int64
			host, m, err = syncFromKeeper(ctx, append(hosts, conf.GetNodesAddr()...), host, h+n, maxBlockID)
			n += m
		}
		if err != nil {
			d.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err, "height": h + n, "host": host}).Error("synchronizing")
			return err
		}
		h += n
	}
	return nil
}

// syncFromKeeper commits the blocks from the height from up to the height to at least by one of
// the hosts but skip that keeps them, and returns it with how many it committed
func syncFromKeeper(ctx context.Context, hosts []string, skip string, from, to int64) (string, int64, error) {
	tried := map[string]bool{skip: true}
	for _, host := range hosts {
		if tried[host] {
			continue
		}
		tried[host] = true
		n, err := syncCertified(ctx, host, from, to)
		if n == 0 && errors.Is(err, tcpclient.ErrNotKept) {
			continue
		}
		return host, n, err
	}
	return skip, 0, fmt.Errorf("%w by any node: height %d", tcpclient.ErrNotKept, from)
}

// syncCertified commits the blocks of host from the height from up to the height to at least,
// each by its certificate, and returns how many it committed
func syncCertified(ctx context.Context, host string, from, to int64) (int64, error) {
	return playBatches(ctx, host, from, to, true, func(b batch) (int64, error) {
		var n int64
		for raw := range b.blocks {
			if n == b.count {
				break
			}
			if err := commitSynced(uint64(b.from+n), b.certs[n], raw); err != nil {
				return n, err
			}
			n++
		}
		return n, nil
	})
}

// commitSynced commits a block of a height by its certificate
func commitSynced(height uint64, data, raw []byte) error {
	cert, err := syncedCertificate(height, data)
	if err != nil {
		return err
	}
	_, err = block.Commit(raw, cert)
	return err
}

// syncedCertificate decodes the certificate a peer sent for a height and verifies its precommits
// against the validators of the height; block.Certify checks that it is of the block
func syncedCertificate(height uint64, data []byte) (*bft.Certificate, error) {
	cert, err := bft.UnmarshalCertificate(data)
	if err != nil {
		return nil, err
	}
	set, err := validatorsAt(height)
	if err != nil {
		return nil, err
	}
	if err := cert.Verify(set, uint64(conf.Config.LocalConf.NetworkID), height, cert.Hash); err != nil {
		return nil, fmt.Errorf("the certificate of height %d: %w", height, err)
	}
	return cert, nil
}
