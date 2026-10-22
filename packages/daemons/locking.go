/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

var (
	mutex = sync.Mutex{}
	// lockedAt is when the lock was taken; only the holder of the lock reads and writes it
	lockedAt time.Time
)

// WaitDB waits for the end of the installation
func WaitDB(ctx context.Context) error {
	// There is could be the situation when installation is not over yet.
	// Database could be created but tables are not inserted yet

	if sqldb.DBConn != nil && CheckDB() {
		return nil
	}

	// poll a base with period
	tick := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-tick.C:
			if sqldb.DBConn != nil && CheckDB() {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// CheckDB check if installation complete or not
func CheckDB() bool {
	install := &sqldb.Install{}

	err := install.Get()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting install")
	}

	if install.Progress == sqldb.ProgressComplete {
		return true
	}

	return false
}

// DBLock locks daemons
func DBLock() {
	start := time.Now()
	mutex.Lock()
	lockedAt = time.Now()
	metrics.LockWait.Observe(lockedAt.Sub(start).Seconds())
}

// DBUnlock unlocks database
func DBUnlock() {
	metrics.LockHold.Observe(metrics.Since(lockedAt))
	mutex.Unlock()
}
