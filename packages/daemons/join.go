/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/network/snapnet"
	"github.com/IBAX-io/go-ibax/packages/network/tcpclient"
	"github.com/IBAX-io/go-ibax/packages/snapshot"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	log "github.com/sirupsen/logrus"
)

// The join of a node from a snapshot of the state (design 4.5). A node of a chain of the BFT
// consensus that has block 1 alone takes the state of the latest snapshot height S its peers keep
// instead of executing the blocks up to it:
//
//  1. it walks the validators sets from the set block 1 leaves up to the height S+D, D the delay
//     of the state hash, by the proofs of the changes: each the block before a change, whose
//     header has the hash of the new set, with its certificate by the set before;
//  2. it verifies the block S+D by its certificate and the set of S+D: the block carries the
//     state hash of S;
//  3. it downloads the snapshot of S, each chunk checked against the manifest and the manifest
//     against the state hash, imports it and checks the hash of what it imported;
//  4. it takes the blocks of the window up to S, each by its certificate and the set of its
//     height, which the state has, so that it serves them and the contracts read them as on the
//     other nodes, and keeps the proofs of the changes out of the window;
//  5. it goes on from S+1 as any node, by the certificates.
//
// It trusts the validators of the sets it walks: the proofs are of more than two thirds of the
// set of each height, so a set out of date, more than two thirds of whose validators sign another
// history, would make another one (the long-range assumption). An archive node executes every
// block instead, to have every row.

// The bounds of the join: the time of a request and the requests at once
const (
	joinTimeout = 30 * time.Second
	joinWorkers = 4
)

// The waits for the state hash of the latest snapshot, that tests shorten: the interval of the
// polls of the tip, and how long the tip may stay before the join gives up waiting
var (
	joinPoll  = time.Second
	joinStall = joinTimeout
)

// The requests of the join, that tests replace
var (
	fetchSnapshotHeights = snapnet.Heights
	fetchSetProofs       = snapnet.FetchProofs
	hostWithMaxBlock     = tcpclient.HostWithMaxBlock
)

// walkedSet is a validators set of the heights from from on
type walkedSet struct {
	from int64
	set  bft.Validators
}

// setAt is the set of a height of the sets walked
func setAt(sets []walkedSet, height int64) bft.Validators {
	i, _ := slices.BinarySearchFunc(sets, height+1, func(s walkedSet, h int64) int { return int(s.from - h) })
	return sets[i-1].set
}

// joiner is the join of a node from the hosts
type joiner struct {
	ctx       context.Context
	logger    *log.Entry
	hosts     []string
	networkID uint64
	delay     int64
	// heights are the snapshot heights of the hosts, with the hosts that keep each
	heights map[int64][]string
}

// joinFromSnapshot joins the node from a snapshot if it can and returns whether it did: a node of
// a chain of the BFT consensus, not an archive, with block 1 alone, whose peers keep a snapshot
// with a block after it that carries its state hash. It joins from the latest snapshot: when the
// block of its state hash is not yet, it waits for it, since the boundary of the snapshot retired
// the start of the window of the ones before it. A join that imported a state and failed is
// an error: the state of the node is no longer that of block 1. restore puts back the state
// outside the database as before block 1, which the state of the join replaces.
func joinFromSnapshot(ctx context.Context, logger *log.Entry, restore func() error) (bool, error) {
	data, ok := block.GetDataFromFirstBlock()
	if !ok {
		return false, nil
	}
	syspar.SetFirstBlockData(data)
	interval, delay := syspar.SnapshotHeights()
	if !syspar.IsBftConsensus() || interval <= 0 || conf.Config.Storage.Archive {
		return false, nil
	}
	if err := syspar.SysUpdate(nil); err != nil {
		return false, err
	}
	j := &joiner{ctx: ctx, logger: logger, networkID: uint64(conf.Config.LocalConf.NetworkID), delay: delay,
		heights: map[int64][]string{}}
	for _, h := range append(conf.GetNodesAddr(), syspar.GetRemoteHosts()...) {
		if !slices.Contains(j.hosts, h) {
			j.hosts = append(j.hosts, h)
		}
	}
	_, tip, err := hostWithMaxBlock(ctx, j.hosts)
	if err != nil || len(j.hosts) == 0 {
		logger.WithError(err).Warn("no host to join from: executing the blocks")
		return false, nil
	}
	heights := j.snapshotHeights(interval)
	if len(heights) > 0 && heights[0]+delay > tip {
		// the boundary of the latest snapshot retired the start of the window of the ones
		// before it: the join waits for the block that carries its state hash
		if tip, err = j.waitTip(heights[0]+delay, tip); err != nil {
			return false, err
		}
	}
	candidates := slices.DeleteFunc(heights, func(s int64) bool { return s+delay > tip })
	if len(candidates) == 0 {
		logger.WithField("tip", tip).Info("no snapshot to join from: executing the blocks")
		return false, nil
	}
	local, err := validatorsAt(2)
	if err != nil {
		return false, err
	}
	imported := false
	var errs []error
	for _, host := range j.hosts {
		sets, proofs, err := j.walk(host, local, candidates[0]+delay)
		if err != nil {
			errs = append(errs, fmt.Errorf("walking the validators sets of %s: %w", host, err))
			continue
		}
		for _, s := range candidates {
			hash, err := j.certifiedStateHash(s+delay, setAt(sets, s+delay))
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if hash == nil {
				continue
			}
			if err := j.join(s, hash, proofs, &imported); err != nil {
				errs = append(errs, fmt.Errorf("joining from snapshot %d: %w", s, err))
				continue
			}
			logger.WithFields(log.Fields{"height": s, "hash": fmt.Sprintf("%x", hash)}).Info("joined from the snapshot")
			return true, restore()
		}
	}
	if imported {
		return false, errors.Join(append([]error{errors.New("no snapshot imported is of its state hash")}, errs...)...)
	}
	logger.WithError(errors.Join(errs...)).Warn("no snapshot to join from: executing the blocks")
	return false, nil
}

// waitTip waits for a host to have the block of a height, while the tip of the hosts moves on
// from tip; it returns the tip it reached, short of the height if the hosts stopped for
// joinStall
func (j *joiner) waitTip(height, tip int64) (int64, error) {
	j.logger.WithFields(log.Fields{"height": height, "tip": tip}).Info("waiting for the state hash of the latest snapshot")
	moved := time.Now()
	for tip < height && time.Since(moved) < joinStall {
		select {
		case <-j.ctx.Done():
			return tip, j.ctx.Err()
		case <-time.After(joinPoll):
		}
		_, t, err := hostWithMaxBlock(j.ctx, j.hosts)
		if err == nil && t > tip {
			tip, moved = t, time.Now()
		}
	}
	return tip, nil
}

// snapshotHeights lists the snapshot heights of the hosts, descending
func (j *joiner) snapshotHeights(interval int64) []int64 {
	for _, h := range j.hosts {
		heights, err := fetchSnapshotHeights(h, joinTimeout)
		if err != nil {
			j.logger.WithError(err).WithField("host", h).Debug("snapshot heights")
			continue
		}
		for _, s := range heights {
			if s%interval == 0 {
				j.heights[s] = append(j.heights[s], h)
			}
		}
	}
	var out []int64
	for s := range j.heights {
		out = append(out, s)
	}
	slices.Sort(out)
	slices.Reverse(out)
	return out
}

// walk walks the validators sets of a host from the set of height 2 up to the height to, each by
// its proof, and returns the sets and the proofs
func (j *joiner) walk(host string, set bft.Validators, to int64) ([]walkedSet, []sqldb.BftSetProof, error) {
	sets := []walkedSet{{from: 2, set: set}}
	var proofs []sqldb.BftSetProof
	for {
		last := sets[len(sets)-1]
		ps, err := fetchSetProofs(host, last.from, snapnet.MaxProofs, joinTimeout)
		if err != nil {
			return nil, nil, err
		}
		for _, p := range ps {
			if p.FromHeight > to {
				return sets, proofs, nil
			}
			cert, err := bft.UnmarshalCertificate(p.Cert)
			if err != nil {
				return nil, nil, err
			}
			b, err := block.VerifyCertified(p.Block, cert, sets[len(sets)-1].set, j.networkID)
			if err != nil {
				return nil, nil, err
			}
			if b.Header.BlockId != p.FromHeight-1 || !bytes.Equal(b.Header.NextValidatorsHash, crypto.Hash(p.Set)) {
				return nil, nil, fmt.Errorf("the proof of the set of height %d is of another set", p.FromHeight)
			}
			next, err := bft.UnmarshalValidators(p.Set)
			if err != nil {
				return nil, nil, err
			}
			sets = append(sets, walkedSet{from: p.FromHeight, set: next})
			proofs = append(proofs, sqldb.BftSetProof{Height: b.Header.BlockId, Block: p.Block, Cert: p.Cert})
		}
		if len(ps) < snapnet.MaxProofs {
			return sets, proofs, nil
		}
	}
}

// certifiedStateHash takes the block of a height from the hosts and verifies it by its
// certificate and the set of the height, and returns the state hash it carries, nil if none
func (j *joiner) certifiedStateHash(height int64, set bft.Validators) ([]byte, error) {
	var errs []error
	for _, host := range j.hosts {
		b, err := j.certifiedBlock(host, height, set)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", host, err))
			continue
		}
		return b.Header.StateHash, nil
	}
	return nil, errors.Join(append([]error{fmt.Errorf("no host has the block %d of its certificate", height)}, errs...)...)
}

// certifiedBlock takes a block of a height from a host and verifies it by its certificate
func (j *joiner) certifiedBlock(host string, height int64, set bft.Validators) (*types.BlockData, error) {
	ctx, cancel := context.WithCancel(j.ctx)
	defer cancel()
	blocks, n, err := fetchBlocks(ctx, host, height, false)
	if err != nil {
		return nil, err
	}
	raw, ok := <-blocks
	if n == 0 || !ok {
		return nil, fmt.Errorf("no block %d", height)
	}
	certs, err := fetchCertificates(host, uint64(height), 1, joinTimeout)
	if err != nil {
		return nil, err
	}
	if len(certs) != 1 {
		return nil, fmt.Errorf("no certificate of height %d", height)
	}
	cert, err := bft.UnmarshalCertificate(certs[0])
	if err != nil {
		return nil, err
	}
	return block.VerifyCertified(raw, cert, set, j.networkID)
}

// join downloads the snapshot of a height of the state hash, imports it, takes the blocks of the
// window and keeps the proofs out of it
func (j *joiner) join(height int64, hash []byte, proofs []sqldb.BftSetProof, imported *bool) error {
	store := snapshot.Store{Dir: filepath.Join(conf.Config.DirPathConf.DataDir, snapshot.DirName)}
	start := time.Now()
	d := &snapnet.Download{Peers: j.heights[height], Height: height, StateHash: hash, Store: store,
		Timeout: joinTimeout, Workers: joinWorkers}
	if _, err := d.Run(j.ctx); err != nil {
		return err
	}
	downloaded := time.Now()
	db, err := sqldb.SnapshotDB(conf.Config.DB)
	if err != nil {
		return err
	}
	defer db.Close()
	*imported = true
	if err := snapshot.Import(j.ctx, db, store, height, hash, joinWorkers); err != nil {
		return err
	}
	if err := syspar.SysUpdate(nil); err != nil {
		return err
	}
	if err := syspar.SysTableColType(nil); err != nil {
		return err
	}
	importedAt := time.Now()
	dbTx, err := sqldb.StartTransaction()
	if err != nil {
		return err
	}
	defer dbTx.Rollback()
	first, err := sqldb.PrepareJoined(dbTx)
	if err != nil {
		return err
	}
	if err := j.window(dbTx, first, height); err != nil {
		return err
	}
	kept := slices.DeleteFunc(proofs, func(p sqldb.BftSetProof) bool { return p.Height >= first })
	if err := sqldb.SaveSetProofs(dbTx, kept); err != nil {
		return err
	}
	if err := (&sqldb.BftStateHash{Height: height, Hash: hash}).SaveIn(dbTx); err != nil {
		return err
	}
	if err := (&sqldb.SnapshotJoin{Height: height, Hash: hash}).Create(dbTx); err != nil {
		return err
	}
	if err := dbTx.Commit(); err != nil {
		return err
	}
	j.logger.WithFields(log.Fields{"height": height, "window": height - first + 1, "proofs": len(kept),
		"download": downloaded.Sub(start), "import": importedAt.Sub(downloaded),
		"blocks": time.Since(importedAt)}).Info("join")
	return nil
}

// window takes the blocks from the height from up to the height to from the hosts, each by its
// certificate and the set of its height, and stores them: the last is the block of the state
func (j *joiner) window(dbTx *sqldb.DbTransaction, from, to int64) error {
	var prev *types.BlockHeader
	next := from
	var errs []error
	for _, host := range j.hosts {
		if next > to {
			break
		}
		_, err := playBatches(j.ctx, host, next, to, true, func(b batch) (int64, error) {
			var blocks []joinedBlock
			var certs []sqldb.BftCertificate
			var n int64
			last := prev
			for raw := range b.blocks {
				if n == b.count {
					break
				}
				h := b.from + n
				if h > to {
					// the blocks of the batch after the state are the synchronization's
					n++
					continue
				}
				bc, c, err := j.windowBlock(h, raw, b.certs[n], last)
				if err != nil {
					// the batch is taken again whole, of another host
					return 0, err
				}
				blocks, certs = append(blocks, *bc), append(certs, *c)
				last = bc.header
				n++
			}
			if err := sqldb.InsertJoined(dbTx, rowsOf(blocks), certs); err != nil {
				return 0, err
			}
			prev, next = last, min(b.from+n, to+1)
			return n, nil
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", host, err))
		}
	}
	if next <= to {
		return errors.Join(append([]error{fmt.Errorf("no host has the blocks from %d", next)}, errs...)...)
	}
	info := &sqldb.InfoBlock{}
	if _, err := info.Get(); err != nil {
		return err
	}
	if info.BlockID != to || !bytes.Equal(info.Hash, prev.BlockHash) || !bytes.Equal(info.RollbacksHash, prev.RollbacksHash) {
		return fmt.Errorf("the state is of block %d %x, not of block %d %x", info.BlockID, info.Hash, to, prev.BlockHash)
	}
	return nil
}

// joinedBlock is a block of the window and its header
type joinedBlock struct {
	sqldb.BlockChain
	header *types.BlockHeader
}

func rowsOf(blocks []joinedBlock) []sqldb.BlockChain {
	rows := make([]sqldb.BlockChain, len(blocks))
	for i, b := range blocks {
		rows[i] = b.BlockChain
	}
	return rows
}

// windowBlock verifies a block of the window by its certificate and the set of its height, after
// the block before if any, and returns its rows
func (j *joiner) windowBlock(height int64, raw, certData []byte, prev *types.BlockHeader) (*joinedBlock, *sqldb.BftCertificate, error) {
	cert, err := bft.UnmarshalCertificate(certData)
	if err != nil {
		return nil, nil, err
	}
	set, err := validatorsAt(uint64(height))
	if err != nil {
		return nil, nil, err
	}
	b, err := block.VerifyCertified(raw, cert, set, j.networkID)
	if err != nil {
		return nil, nil, err
	}
	h := b.Header
	if h.BlockId != height || prev != nil && !bytes.Equal(b.PrevHeader.BlockHash, prev.BlockHash) {
		return nil, nil, fmt.Errorf("block %d does not follow block %d", h.BlockId, height-1)
	}
	bc := &joinedBlock{BlockChain: sqldb.BlockChain{
		ID:             height,
		Hash:           h.BlockHash,
		Data:           raw,
		EcosystemID:    h.EcosystemId,
		KeyID:          h.KeyId,
		NodePosition:   h.NodePosition,
		Time:           h.Timestamp,
		RollbacksHash:  h.RollbacksHash,
		Tx:             int32(len(b.TxFullData)),
		ConsensusMode:  h.ConsensusMode,
		CandidateNodes: h.CandidateNodes,
	}, header: h}
	c := &sqldb.BftCertificate{Height: height, Round: int64(cert.Round), Hash: cert.Hash[:], Data: certData}
	return bc, c, nil
}
