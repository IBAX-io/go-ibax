package model

import (
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/vmihailenco/msgpack/v5"
)

type MineStakeCount struct {
	Devid         int64 `gorm:";not null" ` //ID
	Keyid         int64 `gorm:"not null" `  //ID
	Minerid       int64 `gorm:"not null"`
	Pminerid      int64 `gorm:"not null"`
	Poolid        int64 `gorm:"not null"`
	Status        int64 `gorm:"null"`            //" 0 not  1 over  2 reviewing  3 reviewd"
	Online        int64 `gorm:"null default 0" ` //review 0not review  1 review not over  2reviewd
	Mine_capacity int64 `gorm:"null default 0" ` //
	Count         int64 `gorm:"null default 0" ` //count
	TotalCount    int64 `gorm:"not null" `       //total count
	Stime         int64 `gorm:"not null" `       //stime
	Etime         int64 `gorm:"not null" `       //etime
}

//MintCount example
type MinterCount struct {
	Keyid      int64
	Mineid     int64
	Devid      int64
	Capacity   int64
	Nonce      int64
	BlockId    int64
	Hash       []byte
	MintMap    map[int64]int64
	MineCounts []MineCount
	Time       int64
}

//MintCount example
type MintCount struct {
	Keyid      int64
	Mineid     int64
	Devid      int64
	Capacity   int64
	Nonce      int64
	BlockId    int64
	Rate       int64
	Poolid     int64
	Amount     string
	Foundation string
	Hash       []byte
	PoolInfos  []MinePoolInfo
	MineCounts []MineStakeCount
	Time       int64
}

var MintPrefix = "mint-"

func (m *MintCount) Marshal() ([]byte, error) {
	if res, err := msgpack.Marshal(m); err != nil {
		return nil, err
	} else {
		return res, err
	}
}

func (m *MintCount) Unmarshal(bt []byte) error {
	if err := msgpack.Unmarshal(bt, &m); err != nil {
		return err
	}
	return nil
}

func (m *MinterCount) Changes(dbt *DbTransaction) (*MintCount, error) {
	var miners []MineStakeCount
	mc := MintCount{
		Keyid:    m.Keyid,
		Mineid:   m.Mineid,
		Devid:    m.Devid,
		Capacity: m.Capacity,
		Nonce:    m.Nonce,
		BlockId:  m.BlockId,
		Hash:     m.Hash,
		Time:     m.Time,
	}
	if m.Devid == 0 {
		return &mc, nil
	}

	var ap AppParam
	am, err := ap.GetHvlvebalance(dbt, mc.BlockId)
	if err != nil {
		return &mc, err
	}

	fm, err := ap.GetFoundationbalance(dbt)
	if err != nil {
		return &mc, err
	}

	var sp SystemParameter
	rate, err := sp.GetPoolBlockRate(dbt)
	if err == nil {
		mc.Rate = rate
	}

	var mi MineOwner
	gpm, err := mi.GetAllPoolManage(dbt, m.Time)
	if err != nil {
		return &mc, err
	dMP := m.MintMap
	for i := 0; i < len(m.MineCounts); i++ {
		md := MineStakeCount{
			Devid:         dMC[i].Devid,
			Keyid:         dMC[i].Keyid,
			Minerid:       dMC[i].Minerid,
			Poolid:        dMC[i].Poolid,
			Status:        dMC[i].Status,
			Online:        dMC[i].Online,
			Mine_capacity: dMC[i].MineCapacity,
			Count:         dMC[i].Count,
			Stime:         dMC[i].Stime,
			Etime:         dMC[i].Etime,
		}

		if dMC[i].Devid == mc.Devid {
			mc.Poolid = dMC[i].Poolid
			if mc.Poolid != 0 {
				mid, ok := gpm[mc.Poolid]
				if ok {
					mc.Mineid = mid
					mc.Keyid = mid
				}

				drate := float64(rate) / float64(100)
				dr := 1 - drate
				if dr > 0 {
					dm := decimal.NewFromFloat(dr)
					am = am.Mul(dm)
				}
			} else {
				mc.Mineid = dMC[i].Minerid
				mc.Keyid = dMC[i].Keyid
			}
		}

		if md.Poolid != 0 {
			v, ok := gpm[md.Poolid]
			if ok {
				md.Pminerid = v
			}
		}

		v, ok := dMP[dMC[i].Devid]
		if ok {
			md.TotalCount = v
		}
		miners = append(miners, md)
	}

	if mc.Poolid != 0 {

	}
	mc.Amount = am.String()
	mc.Foundation = fm.String()
	mc.MineCounts = miners
	return &mc, nil
}

func (m *MintCount) Get(id int64) (bool, error) {
	rp := &RedisParams{
		Key: MintPrefix + strconv.FormatInt(id, 10),
	}
	for i := 0; i < 10; i++ {
		err := rp.Getdb()
		if err == nil {
			err = m.Unmarshal([]byte(rp.Value))
			return true, err
		}
		if err.Error() == "redis: nil" {
			break
		} else {
			time.Sleep(200 * time.Millisecond)
		}

	}

	return false, nil
}

func (m *MinterCount) Insert_redisdb(dbt *DbTransaction) error {
	mc, err := m.Changes(dbt)
	if err != nil {
		return err
	}
	val, err := mc.Marshal()
	if err != nil {
		return err
	}
	rp := RedisParams{
		Key:   MintPrefix + strconv.FormatInt(m.BlockId, 10),
		Value: string(val),
	}
	return rp.Setdb()
}
