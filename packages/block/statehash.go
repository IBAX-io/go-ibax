/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/snapshot"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	log "github.com/sirupsen/logrus"
)

// stateStatus is what a node knows of the state hash of a snapshot height
type stateStatus int

const (
	// stateUnknown: the node took no snapshot of the height, or lost it before its hash
	stateUnknown stateStatus = iota
	// statePending: the node hashes the snapshot of the height
	statePending
	// stateDone: the node has the hash of the height
	stateDone
)

const (
	// snapshotWorkers is the tables a snapshot encodes at once
	snapshotWorkers = 4
	// keptSnapshots is the snapshots whose files a node keeps, the latest
	keptSnapshots = 2
)

// stateHashes takes the snapshots of the state of a chain of the BFT consensus and hashes them
// (design 4.3): after the commit of each block of a height the interval divides, it takes the
// snapshot of the state the commit left, before the next block commits, and hashes it in the
// background, writing the files of the snapshot under dir. A hash is kept in bft_state_hashes;
// the files of the latest keptSnapshots snapshots are kept.
type stateHashes struct {
	mu sync.Mutex
	// open opens the database of the snapshots and names the directory of their files, at the
	// first snapshot
	open    func() (*sql.DB, string, error)
	db      *sql.DB
	dir     string
	pending map[int64]chan struct{}
}

var states = &stateHashes{open: func() (*sql.DB, string, error) {
	db, err := sqldb.SnapshotDB(conf.Config.DB)
	return db, filepath.Join(conf.Config.DirPathConf.DataDir, snapshot.DirName), err
}}

// stateHeight is the snapshot height whose state hash the block of a height carries, 0 if none:
// a block delay heights after a snapshot carries its hash
func stateHeight(height int64) int64 {
	interval, delay := syspar.SnapshotHeights()
	if interval <= 0 {
		return 0
	}
	s := height - delay
	if s < interval || s%interval != 0 {
		return 0
	}
	return s
}

// isSnapshotHeight reports whether the node takes a snapshot after the commit of a height
func isSnapshotHeight(height int64) bool {
	interval, _ := syspar.SnapshotHeights()
	return interval > 0 && height > 0 && height%interval == 0
}

// afterCommit takes the snapshot of the height of a block just committed if the interval divides
// it. It runs before the next block commits, under the lock of the blocks; a snapshot it fails to
// take leaves the hash of the height unknown.
func (s *stateHashes) afterCommit(height int64) {
	if !isSnapshotHeight(height) {
		return
	}
	logger := log.WithFields(log.Fields{"block_id": height})
	s.mu.Lock()
	if s.db == nil {
		db, dir, err := s.open()
		if err != nil {
			s.mu.Unlock()
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("opening the database of the snapshots")
			return
		}
		s.db, s.dir = db, dir
	}
	db := s.db
	if s.pending == nil {
		s.pending = map[int64]chan struct{}{}
	}
	if _, ok := s.pending[height]; ok {
		s.mu.Unlock()
		return
	}
	snap, err := snapshot.Begin(context.Background(), db)
	if err != nil {
		s.mu.Unlock()
		logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("taking the snapshot of the state")
		return
	}
	done := make(chan struct{})
	s.pending[height] = done
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.pending, height)
			s.mu.Unlock()
			close(done)
		}()
		start := time.Now()
		state, err := s.hash(snap, height)
		if err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("hashing the snapshot of the state")
			return
		}
		if err := (&sqldb.BftStateHash{Height: height, Hash: state.Hash}).Save(); err != nil {
			logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("saving the state hash")
			return
		}
		logger.WithFields(log.Fields{"hash": fmt.Sprintf("%x", state.Hash), "tables": len(state.Tables),
			"duration": time.Since(start)}).Info("state hash")
	}()
}

// hash hashes the snapshot of a height and writes its files (snapshot.Store), keeping those of
// the latest keptSnapshots snapshots
func (s *stateHashes) hash(snap *snapshot.Snapshot, height int64) (*snapshot.State, error) {
	defer snap.Close()
	if s.dir == "" {
		return snap.Hash(context.Background(), snapshotWorkers, nil)
	}
	store := snapshot.Store{Dir: s.dir}
	files, err := store.Create(height)
	if err != nil {
		return nil, err
	}
	state, err := snap.Hash(context.Background(), snapshotWorkers, files.Sink())
	if err == nil {
		err = files.Commit(state)
	}
	if err != nil {
		files.Abort()
		return nil, err
	}
	return state, store.Prune(keptSnapshots, s.isPending)
}

func (s *stateHashes) isPending(height int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.pending[height]
	return ok
}

// get is what the node knows of the state hash of a height, and the hash if it has it
func (s *stateHashes) get(height int64) ([]byte, stateStatus, error) {
	s.mu.Lock()
	done, ok := s.pending[height]
	s.mu.Unlock()
	if ok {
		select {
		case <-done:
		default:
			return nil, statePending, nil
		}
	}
	h := &sqldb.BftStateHash{}
	found, err := h.Get(height)
	if err != nil || !found {
		return nil, stateUnknown, err
	}
	return h.Hash, stateDone, nil
}

// wait is get once the node has hashed the snapshot of a height it hashes
func (s *stateHashes) wait(height int64) ([]byte, stateStatus, error) {
	s.mu.Lock()
	done, ok := s.pending[height]
	s.mu.Unlock()
	if ok {
		<-done
	}
	return s.get(height)
}
