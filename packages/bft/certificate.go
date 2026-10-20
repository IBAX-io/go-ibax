/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// ErrCertificate is a commit certificate that does not prove the commit of its block
var ErrCertificate = errors.New("bft: the certificate does not commit the block")

// Signed is the precommit of a validator in a certificate; the type, height, round and hash are
// the certificate's
type Signed struct {
	Validator uint32
	Signature []byte
}

// Certificate is the proof of the commit of a block: the precommits of a quorum of the
// validators of its height for its hash in one round, in ascending order of the validators
type Certificate struct {
	Height     uint64
	Round      uint32
	Hash       Hash
	Precommits []Signed
}

// CertificateOf is the certificate of a commit
func CertificateOf(c *Commit) *Certificate {
	cert := &Certificate{Height: c.Height, Round: c.Round, Hash: c.Hash}
	for _, v := range c.Precommits {
		cert.Precommits = append(cert.Precommits, Signed{Validator: v.Validator, Signature: v.Signature})
	}
	return cert
}

// Marshal is the stored and wire form of a certificate:
// height u64 ‖ round u32 ‖ hash 32 ‖ count u16 ‖ count × (validator u32 ‖ len u16 ‖ signature)
func (c *Certificate) Marshal() []byte {
	if len(c.Precommits) > math.MaxUint16 {
		panic("bft: a certificate of more than a uint16 of precommits")
	}
	b := make([]byte, 0, 8+4+HashSize+2+len(c.Precommits)*(4+2+64))
	b = binary.BigEndian.AppendUint64(b, c.Height)
	b = binary.BigEndian.AppendUint32(b, c.Round)
	b = append(b, c.Hash[:]...)
	b = binary.BigEndian.AppendUint16(b, uint16(len(c.Precommits)))
	for _, s := range c.Precommits {
		b = binary.BigEndian.AppendUint32(b, s.Validator)
		b = appendBytes16(b, s.Signature)
	}
	return b
}

// UnmarshalCertificate decodes a certificate; it refuses the hash of nil, no precommits,
// validators out of ascending order and bytes left over
func UnmarshalCertificate(b []byte) (*Certificate, error) {
	r := reader{b: b}
	c := &Certificate{Height: r.u64(), Round: r.u32(), Hash: r.hash()}
	n := int(r.u16())
	for i := 0; i < n && r.err == nil; i++ {
		s := Signed{Validator: r.u32(), Signature: r.bytes16()}
		if r.err == nil && i > 0 && s.Validator <= c.Precommits[i-1].Validator {
			return nil, fmt.Errorf("%w: precommits out of the order of the validators", ErrFormat)
		}
		c.Precommits = append(c.Precommits, s)
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	if c.Hash == Nil {
		return nil, fmt.Errorf("%w: a certificate of nil", ErrFormat)
	}
	if n == 0 {
		return nil, fmt.Errorf("%w: a certificate of no precommits", ErrFormat)
	}
	return c, nil
}

// Verify checks that the certificate commits the block hash at the height: every precommit is of
// a distinct validator of the set and verifies, and they are a quorum of the set. One precommit
// that fails fails the certificate.
func (c *Certificate) Verify(set Validators, networkID, height uint64, hash Hash) error {
	if c.Height != height || c.Hash != hash {
		return fmt.Errorf("%w: it is of the height %d and the hash %x", ErrCertificate, c.Height, c.Hash)
	}
	for i, s := range c.Precommits {
		if i > 0 && s.Validator <= c.Precommits[i-1].Validator {
			return fmt.Errorf("%w: precommits out of the order of the validators", ErrCertificate)
		}
		key, ok := set.Key(s.Validator)
		if !ok {
			return fmt.Errorf("%w: %d is no validator", ErrCertificate, s.Validator)
		}
		if err := Verify(key, networkID, c.Height, c.Round, TypePrecommit, c.Hash, NoRound, s.Signature); err != nil {
			return fmt.Errorf("%w: the precommit of %d: %w", ErrCertificate, s.Validator, err)
		}
	}
	if len(c.Precommits) < set.Quorum() {
		return fmt.Errorf("%w: %d precommits of a quorum of %d", ErrCertificate, len(c.Precommits), set.Quorum())
	}
	return nil
}
