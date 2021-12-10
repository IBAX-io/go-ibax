/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf"
)

type banKey struct {
	Time time.Time   // banned till
	Bad  []time.Time // time of bad tx
}

var (
	banList = make(map[int64]banKey)
	mutex   = &sync.RWMutex{}
)

// IsKeyBanned returns true if the key has been banned
func IsKeyBanned(keyID int64) bool {
	mutex.RLock()
	if ban, ok := banList[keyID]; ok {
		mutex.RUnlock()
		now := time.Now()
		if now.Before(ban.Time) {
			return true
		}
		for i := 0; i < conf.Config.BanKey.BadTx; i++ {
			if ban.Bad[i].Add(time.Duration(conf.Config.BanKey.BadTime) * time.Minute).After(now) {
				return false
			}
		}
		// Delete if time of all bad tx is old
		mutex.Lock()
		delete(banList, keyID)
		mutex.Unlock()
	} else {
		mutex.RUnlock()
	}
	return false
}

// BannedTill returns the time that the user has been banned till
func BannedTill(keyID int64) string {
	mutex.RLock()
	defer mutex.RUnlock()
	if ban, ok := banList[keyID]; ok {
		return ban.Time.Format(`2006-01-02 15:04:05`)
	}
	return ``
}

// BadTxForBan adds info about bad tx of the key
func BadTxForBan(keyID int64) {
	var (
		ban banKey
		ok  bool
	)
	mutex.Lock()
	defer mutex.Unlock()
	now := time.Now()
	if ban, ok = banList[keyID]; ok {
		var bMin, count int
		for i := 0; i < conf.Config.BanKey.BadTx; i++ {
			if ban.Bad[i].Add(time.Duration(conf.Config.BanKey.BadTime) * time.Minute).After(now) {
				count++
			}
			if i > bMin && ban.Bad[i].Before(ban.Bad[bMin]) {
				bMin = i
			}
		}
		ban.Bad[bMin] = now
		if count >= conf.Config.BanKey.BadTx-1 {
			ban.Time = now.Add(time.Duration(conf.Config.BanKey.BanTime) * time.Minute)
		}
	} else {
		ban = banKey{Bad: make([]time.Time, conf.Config.BanKey.BadTx)}
		ban.Bad[0] = time.Now()
	}
	banList[keyID] = ban
}
