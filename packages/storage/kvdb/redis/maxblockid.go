/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package redis

import (
	"github.com/vmihailenco/msgpack/v5"
)

// BlockID is model
type BlockID struct {
	ID   int64
	Time int64
	Name string
}

var MihPrefix = "blockid-"

//marshal
func (b *BlockID) Marshal() ([]byte, error) {
	if res, err := msgpack.Marshal(b); err != nil {
		return nil, err
	} else {
		return res, err
	}
}

//unmarshal
func (b *BlockID) Unmarshal(bt []byte) error {
	if err := msgpack.Unmarshal(bt, &b); err != nil {
		return err
	}
	return nil
}

//Get by name
func (b *BlockID) GetbyName(name string) (bool, error) {
	rp := &RedisParams{
		Key: MihPrefix + name,
	}
	if err := rp.Getdb1(); err != nil {
		return false, err
	}
	if err := b.Unmarshal([]byte(rp.Value)); err != nil {
		return false, err
	}
	return true, nil
}

//Get by name
func (b *BlockID) GetRangeByName(n1, n2 string, count int64) (bool, error) {
	var nb1, nb2 BlockID
	rp1 := &RedisParams{
		Key: MihPrefix + n1,
	}
	if err := rp1.Getdb1(); err != nil {
		if err.Error() == "redis: nil" {
			rp := &RedisParams{}
			num, err := rp.Getdbsize()
			if err != nil {
				return false, err
			}
			if num > count {
				return true, err
			} else {
				return false, err
			}
			//return false, err
		}
		return false, err
	}
	if err := nb1.Unmarshal([]byte(rp1.Value)); err != nil {
		return false, err
	}

	rp2 := &RedisParams{
		Key: MihPrefix + n2,
	}
	if err := rp2.Getdb1(); err != nil {
		return false, err
	}
	if err := nb2.Unmarshal([]byte(rp2.Value)); err != nil {
		return false, err
	}

	if (nb2.ID - nb1.ID) > count {
		return true, nil
	}

	return false, nil
}
