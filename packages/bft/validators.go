/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

// Quorum is the number of votes of a set of n validators that commits: more than two thirds,
// Q(n) = floor(2n/3) + 1
func Quorum(n int) int {
	return 2*n/3 + 1
}

// Faults is the number of faulty validators a set of n bears, f(n) = floor((n-1)/3)
func Faults(n int) int {
	if n < 1 {
		return 0
	}
	return (n - 1) / 3
}

// Validators is the set of validators of a height: the node public keys of the entries of
// honor_nodes that are neither stopped there nor stopped by evidence, in the order of honor_nodes.
// A validator is its index in the set.
type Validators struct {
	Keys [][]byte
}

// N is the number of validators
func (v Validators) N() int {
	return len(v.Keys)
}

// Quorum is the number of votes that commits
func (v Validators) Quorum() int {
	return Quorum(len(v.Keys))
}

// Faults is the number of faulty validators the set bears
func (v Validators) Faults() int {
	return Faults(len(v.Keys))
}

// Proposer is the validator that proposes at height h and round r: (h + r) mod N
func (v Validators) Proposer(h uint64, r uint32) uint32 {
	n := uint64(len(v.Keys))
	return uint32((h%n + uint64(r)%n) % n)
}

// Key is the node public key of a validator, and false if the set has no such validator
func (v Validators) Key(i uint32) ([]byte, bool) {
	if uint64(i) >= uint64(len(v.Keys)) {
		return nil, false
	}
	return v.Keys[i], true
}

// Index is the validator of a node public key, and false if the key is no validator of the set
func (v Validators) Index(key []byte) (uint32, bool) {
	for i, k := range v.Keys {
		if bytes.Equal(k, key) {
			return uint32(i), true
		}
	}
	return 0, false
}

// Marshal is the stored form of the set: count u16 ‖ count × (len u16 ‖ node public key)
func (v Validators) Marshal() []byte {
	if len(v.Keys) > math.MaxUint16 {
		panic("bft: a set of more than a uint16 of validators")
	}
	b := binary.BigEndian.AppendUint16(nil, uint16(len(v.Keys)))
	for _, k := range v.Keys {
		b = appendBytes16(b, k)
	}
	return b
}

// UnmarshalValidators decodes a set; it refuses an empty key, a key twice and bytes left over
func UnmarshalValidators(b []byte) (Validators, error) {
	r := reader{b: b}
	n := int(r.u16())
	var v Validators
	for i := 0; i < n && r.err == nil; i++ {
		k := r.bytes16()
		if r.err != nil {
			break
		}
		if len(k) == 0 {
			return Validators{}, fmt.Errorf("%w: an empty key of a validator", ErrFormat)
		}
		if _, ok := v.Index(k); ok {
			return Validators{}, fmt.Errorf("%w: a validator twice", ErrFormat)
		}
		v.Keys = append(v.Keys, k)
	}
	if err := r.done(); err != nil {
		return Validators{}, err
	}
	return v, nil
}
