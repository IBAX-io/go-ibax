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

// An execution status decodes to itself and verifies by the key of its validator only, on its
// network only; no byte of its wire form can change and still verify; it is not signed as a
// message of the consensus
func TestExecStatus(t *testing.T) {
	vs, set := testSet(t, 4)
	s := &ExecStatus{Validator: 1, Height: 951, Rate: 2056}
	if err := s.Sign(vs[1].private, testNetwork); err != nil {
		t.Fatal(err)
	}
	wire := s.Marshal()
	got, err := UnmarshalExecStatus(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Marshal(), wire) {
		t.Fatal("the execution status does not decode to itself")
	}
	key, _ := set.Key(1)
	if err := got.Verify(key, testNetwork); err != nil {
		t.Fatal(err)
	}
	if err := got.Verify(key, testNetwork+1); !errors.Is(err, ErrSignature) {
		t.Fatalf("an execution status of another network: %v", err)
	}
	other, _ := set.Key(2)
	if err := got.Verify(other, testNetwork); !errors.Is(err, ErrSignature) {
		t.Fatalf("an execution status by the key of another validator: %v", err)
	}
	for i := range wire {
		bad := append([]byte(nil), wire...)
		bad[i] ^= 0x01
		w, err := UnmarshalExecStatus(bad)
		if err != nil {
			continue
		}
		key, ok := set.Key(w.Validator)
		if ok && w.Verify(key, testNetwork) == nil {
			t.Errorf("byte %d changed: the execution status verifies", i)
		}
	}
	if _, err := UnmarshalExecStatus(append(wire, 0)); !errors.Is(err, ErrFormat) {
		t.Fatalf("a byte left over: %v", err)
	}
	zero := &ExecStatus{Validator: 1, Rate: 5, Signature: []byte{1}}
	if _, err := UnmarshalExecStatus(zero.Marshal()); !errors.Is(err, ErrFormat) {
		t.Fatalf("an execution status of height 0: %v", err)
	}
	if bytes.HasPrefix(ExecSignBytes(testNetwork, 951, 2056), []byte(MsgPrefix)) {
		t.Fatal("an execution status is signed as a message of the consensus")
	}
}
