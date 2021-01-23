/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package miner

import (
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/crypto"
	"github.com/IBAX-io/go-ibax/packages/model"
	log "github.com/sirupsen/logrus"
)

type minersCount struct {
	start, end, devid int64
}
type mint struct {
	minerPool  []minersCount
	db         *model.DbTransaction
	prevHash   []byte
	time       int64
	blockid    int64
	logger     *log.Entry
	MintMap    map[int64]int64
	MineCounts []model.MineCount
}

func NewMint(dbt *model.DbTransaction, prevHash []byte, time, blockid int64, log *log.Entry) *mint {
	return &mint{
		db:       dbt,
		prevHash: prevHash,
		time:     time,
		blockid:  blockid,
		logger:   log,
	}
}

func (m *mint) MinerTime() (capacities, nonce, devid int64, err error) {
	nonce, capacities, err = m.makeMiningPoolTime()
	if err != nil || nonce == 0 {
		return
	}
	random := crypto.Address(m.prevHash)
	if random < 0 {
		random = -random
	}
				Hash:       m.prevHash,
				Time:       m.time,
			}
			err := mc.Insert_redisdb(m.db)
			if err != nil {
				log.Error("Deal_MintCount Insert_redisdb: ", err.Error())
			}
		}
	}
	return
}

func (m *mint) makeMiningPoolTime() (int64, int64, error) {
	mineCount, miners, capacity, err := m.getMiners()
	if err != nil {
		return 0, capacity, err
	}
	var st int64
	for i := 0; i < len(mineCount); i++ {
		k := miners[i].Devid
		v, ok := mineCount[k]
		if ok {
			da := minersCount{devid: k, start: st, end: st + v}
			st = st + v
			m.minerPool = append(m.minerPool, da)
		}
	}
	m.MintMap = mineCount
	m.MineCounts = miners
	return st, capacity, nil
}

func (m *mint) getMiners() (map[int64]int64, []model.MineCount, int64, error) {
	// mineCount key is devid, value isd that devid sum count
	mineCount := make(map[int64]int64)
	miner := model.MineCount{}
	var capacity int64
	miners, err := miner.GetActiveMiner(m.db, m.time)
	if err != nil {
		return nil, nil, 0, err
	}

	for i := 0; i < len(miners); i++ {
		mine := miners[i]
		mineCount[mine.Devid] = mine.Count
		capacity += mine.MineCapacity
	}

	minerpow := model.MineInvitepowadd{}
	minerpows, err := minerpow.GetALL(m.db, m.time)
	if err != nil {
		return nil, nil, 0, err
	}

	for _, mine := range minerpows {
		v, ok := mineCount[mine.Devid]
		if ok {
			mineCount[mine.Devid] = v + mine.Count
		}
	}
	return mineCount, miners, capacity, err
}

func (m *mint) getMintRandom(remainder int64) int64 {
	num := len(m.minerPool)
	if num == 1 {
		return m.minerPool[0].devid
	}
	end := num - 1
	start := 0
	mid := (end + start) / 2
	for count := 1; count <= num; count++ {
		dat := m.minerPool[mid]
		if remainder >= dat.start && remainder < dat.end {
			return dat.devid
		} else if remainder >= dat.end {
			start = mid + 1
		} else {
			end = mid - 1
		}
		mid = start + (end-start)/2
	}
	return 0
}
