/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"bytes"
	"errors"
	"testing"
)

func testCertificate(t *testing.T, vs []testValidator, height uint64, round uint32, hash Hash, signers ...uint32) *Certificate {
	t.Helper()
	c := &Certificate{Height: height, Round: round, Hash: hash}
	for _, i := range signers {
		v := &Vote{Type: TypePrecommit, Height: height, Round: round, Validator: i, Hash: hash}
		if err := v.Sign(vs[i].private, testNetwork); err != nil {
			t.Fatal(err)
		}
		c.Precommits = append(c.Precommits, Signed{Validator: i, Signature: v.Signature})
	}
	return c
}

func TestCertificate(t *testing.T) {
	vs, set := testSet(t, 4)
	hash := testHash(0x5c)
	c := testCertificate(t, vs, 9, 2, hash, 0, 1, 3)
	b := c.Marshal()
	if len(b) != 8+4+HashSize+2+3*(4+2+64) {
		t.Fatalf("a certificate of 3 ECC_P256 precommits is %d bytes", len(b))
	}
	got, err := UnmarshalCertificate(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Marshal(), b) {
		t.Fatal("the certificate does not decode to itself")
	}
	if err := got.Verify(set, testNetwork, 9, hash); err != nil {
		t.Fatal(err)
	}
	// all four sign too
	if err := testCertificate(t, vs, 9, 2, hash, 0, 1, 2, 3).Verify(set, testNetwork, 9, hash); err != nil {
		t.Fatal(err)
	}

	// every byte past the counts is part of a field the decoding or the check holds to
	for i := range b {
		if i == 8+4+HashSize || i == 8+4+HashSize+1 {
			continue // the count, which fails the decoding by a short read or bytes left over
		}
		bad := bytes.Clone(b)
		bad[i] ^= 0x01
		c, err := UnmarshalCertificate(bad)
		if err != nil {
			continue
		}
		if c.Verify(set, testNetwork, 9, hash) == nil {
			t.Fatalf("the certificate with byte %d changed verifies", i)
		}
	}
	for _, n := range []int{len(b) - 1, len(b) + 1} {
		bad := make([]byte, n)
		copy(bad, b)
		if _, err := UnmarshalCertificate(bad); !errors.Is(err, ErrFormat) {
			t.Fatalf("a certificate of %d bytes: %v", n, err)
		}
	}

	cases := []struct {
		name string
		c    *Certificate
		err  error
	}{
		{"short of a quorum", testCertificate(t, vs, 9, 2, hash, 0, 3), ErrCertificate},
		{"of another height", testCertificate(t, vs, 8, 2, hash, 0, 1, 3), ErrCertificate},
		{"of another hash", testCertificate(t, vs, 9, 2, testHash(0x5d), 0, 1, 3), ErrCertificate},
		{"of no validator", testCertificate(t, vs, 9, 2, hash, 0, 1, 2), nil},
	}
	cases[3].c.Precommits[2].Validator = 4
	cases[3].err = ErrCertificate
	for _, tc := range cases {
		if err := tc.c.Verify(set, testNetwork, 9, hash); !errors.Is(err, tc.err) {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}

	// a precommit of another round, signed by its validator, does not count for the round
	other := testCertificate(t, vs, 9, 3, hash, 2)
	mixed := testCertificate(t, vs, 9, 2, hash, 0, 1)
	mixed.Precommits = append(mixed.Precommits, other.Precommits[0])
	if err := mixed.Verify(set, testNetwork, 9, hash); !errors.Is(err, ErrCertificate) {
		t.Fatalf("a precommit of another round: %v", err)
	}
	// a prevote is no precommit
	prevote := &Vote{Type: TypePrevote, Height: 9, Round: 2, Validator: 3, Hash: hash}
	if err := prevote.Sign(vs[3].private, testNetwork); err != nil {
		t.Fatal(err)
	}
	mixed = testCertificate(t, vs, 9, 2, hash, 0, 1)
	mixed.Precommits = append(mixed.Precommits, Signed{Validator: 3, Signature: prevote.Signature})
	if err := mixed.Verify(set, testNetwork, 9, hash); !errors.Is(err, ErrCertificate) {
		t.Fatalf("a prevote in a certificate: %v", err)
	}
	// one validator twice does not count twice
	twice := testCertificate(t, vs, 9, 2, hash, 0, 1, 1)
	if _, err := UnmarshalCertificate(twice.Marshal()); !errors.Is(err, ErrFormat) {
		t.Fatalf("a validator twice decodes: %v", err)
	}
	if err := twice.Verify(set, testNetwork, 9, hash); !errors.Is(err, ErrCertificate) {
		t.Fatalf("a validator twice verifies: %v", err)
	}
	for name, c := range map[string]*Certificate{
		"of nil":           testCertificate(t, vs, 9, 2, Nil, 0, 1, 3),
		"of no precommits": {Height: 9, Round: 2, Hash: hash},
	} {
		if _, err := UnmarshalCertificate(c.Marshal()); !errors.Is(err, ErrFormat) {
			t.Fatalf("a certificate %s decodes: %v", name, err)
		}
	}
}

func TestValidatorsMarshal(t *testing.T) {
	_, set := testSet(t, 7)
	b := set.Marshal()
	got, err := UnmarshalValidators(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Keys) != 7 || !bytes.Equal(got.Marshal(), b) {
		t.Fatal("the set does not decode to itself")
	}
	if got, err := UnmarshalValidators(Validators{}.Marshal()); err != nil || got.N() != 0 {
		t.Fatalf("the empty set: %v", err)
	}
	for name, bad := range map[string][]byte{
		"short":     b[:len(b)-1],
		"left over": append(bytes.Clone(b), 0),
		"twice":     Validators{Keys: [][]byte{set.Keys[0], set.Keys[0]}}.Marshal(),
		"empty key": Validators{Keys: [][]byte{set.Keys[0], {}}}.Marshal(),
	} {
		if _, err := UnmarshalValidators(bad); !errors.Is(err, ErrFormat) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
