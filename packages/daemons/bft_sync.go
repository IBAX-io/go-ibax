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
	"github.com/IBAX-io/go-ibax/packages/network/bftnet"
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

	DBLock()
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
	for h := last + 1; h <= uint64(maxBlockID); {
		n, err := syncBatch(ctx, host, h)
		if errors.Is(err, tcpclient.ErrNotKept) {
			// the blocks are out of the window of the host: a node that keeps them goes on
			host, n, err = syncFromKeeper(ctx, append(hosts, conf.GetNodesAddr()...), host, h)
		}
		if err != nil {
			d.logger.WithFields(log.Fields{"type": consts.BlockError, "error": err, "height": h + uint64(n), "host": host}).Error("synchronizing")
			return err
		}
		h += uint64(n)
	}
	return nil
}

// syncFromKeeper commits the blocks from the height from by one of the hosts but skip that keeps
// them, and returns it with how many it committed
func syncFromKeeper(ctx context.Context, hosts []string, skip string, from uint64) (string, int, error) {
	tried := map[string]bool{skip: true}
	for _, host := range hosts {
		if tried[host] {
			continue
		}
		tried[host] = true
		n, err := syncBatch(ctx, host, from)
		if errors.Is(err, tcpclient.ErrNotKept) {
			continue
		}
		return host, n, err
	}
	return skip, 0, fmt.Errorf("%w by any node: height %d", tcpclient.ErrNotKept, from)
}

// syncBatch commits up to bftnet.MaxCertificates blocks from the height from, and returns how many
func syncBatch(ctx context.Context, host string, from uint64) (int, error) {
	certs, err := bftnet.FetchCertificates(host, from, bftnet.MaxCertificates, bftFetchTimeout)
	if err != nil {
		return 0, err
	}
	if len(certs) == 0 {
		// the host has blocks after it: the height is out of its window
		return 0, fmt.Errorf("%w: %s has no certificate of height %d", tcpclient.ErrNotKept, host, from)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	bodies, err := tcpclient.GetBlocksBodies(ctx, host, int64(from), false)
	if err != nil {
		return 0, err
	}
	n := 0
	for raw := range bodies {
		if n == len(certs) {
			break
		}
		if err := commitSynced(from+uint64(n), certs[n], raw); err != nil {
			return n, err
		}
		n++
	}
	if n == 0 {
		return 0, fmt.Errorf("%s sent no block of height %d", host, from)
	}
	return n, nil
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
