/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package bft

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"os"
	"path/filepath"
	"unsafe"
)

// SignStateFile is the file of the sign state in the data directory of the node. The sign state
// is of the node key, not of the chain: it stays with the key when the database is restored or
// joined from a snapshot, and the node rewrites it each time it signs, which a table would keep
// as dead rows of the size of the proposals.
const SignStateFile = "bft_sign_state"

// The file has two slots of the same size, which the node overwrites in turn. A slot is
//
//	sequence (8 bytes) | length of the state (4) | CRC-32C of the sequence, the length and the state (4) | state
//
// in little endian. A write overwrites the slot of the older state and syncs its data only: the
// blocks of the file are written when it is made, so the write changes no metadata and waits for
// no commit of the file system's journal, which on a disk busy with the database took up to
// 0.7 s for a rename and the sync of the directory. A crash in a write damages that slot only;
// the other keeps the state before. On Linux the slots are written with O_DIRECT|O_DSYNC, in
// blocks of directBlock: the disk writes the slot through (FUA) and does not flush its whole write
// cache, which fdatasync does: beside the writes of a node's database on a consumer NVMe disk, a
// 128 KiB slot synced with fdatasync took up to 0.64 s, written directly up to 0.10 s. A file
// system that does not take O_DIRECT, as tmpfs before Linux 6.6, keeps fdatasync.
const (
	slotHeader = 16
	// minSlot is the smallest slot, a multiple of the page size, and the slots double from it: the
	// state holds no blocks, and with the proposals and the votes of ML-DSA-65 keys, about 3.4 KB
	// each, a state of a few rounds fits the slot and the file is made once
	minSlot = 64 << 10
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// SignFile is the open file of the sign state
type SignFile struct {
	path string
	f    *os.File
	w    *os.File // the file opened for the direct writes of the slots; nil without them
	slot int64    // size of a slot; 0 before the file is made
	seq  uint64   // sequence of the last state written
}

// OpenSignFile opens the file of the sign state of path and reads its last state; the state is nil
// if the node has none
func OpenSignFile(path string) (*SignFile, *SignState, error) {
	sf := &SignFile{path: path}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return sf, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	b, err := sf.read(f)
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	s, err := UnmarshalSignState(b)
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	sf.f = f
	if sf.w, err = openDirect(path); err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return sf, s, nil
}

// read finds the slot of the last state in f and sets the slot size and the sequence
func (sf *SignFile) read(f *os.File) ([]byte, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size < 2*minSlot || size%(2*minSlot) != 0 {
		return nil, fmt.Errorf("not a file of the sign state: %d bytes", size)
	}
	sf.slot = size / 2
	var last []byte
	found := false
	for i := int64(0); i < 2; i++ {
		buf := make([]byte, sf.slot)
		if _, err := f.ReadAt(buf, i*sf.slot); err != nil {
			return nil, err
		}
		seq, state, ok := parseSlot(buf)
		if ok && (!found || seq > sf.seq) {
			sf.seq, last, found = seq, state, true
		}
	}
	if !found {
		return nil, errors.New("both slots of the sign state are damaged")
	}
	return last, nil
}

// parseSlot is the sequence and the state of a slot, if its checksum holds
func parseSlot(buf []byte) (uint64, []byte, bool) {
	n := int64(binary.LittleEndian.Uint32(buf[8:12]))
	if n > int64(len(buf))-slotHeader {
		return 0, nil, false
	}
	sum := crc32.Update(crc32.Checksum(buf[:12], castagnoli), castagnoli, buf[slotHeader:slotHeader+n])
	if sum != binary.LittleEndian.Uint32(buf[12:16]) {
		return 0, nil, false
	}
	return binary.LittleEndian.Uint64(buf[:8]), buf[slotHeader : slotHeader+n], true
}

// record is a slot of the state b at sequence seq, without the padding
func record(seq uint64, b []byte) []byte {
	r := make([]byte, slotHeader+len(b))
	binary.LittleEndian.PutUint64(r[:8], seq)
	binary.LittleEndian.PutUint32(r[8:12], uint32(len(b)))
	copy(r[slotHeader:], b)
	sum := crc32.Update(crc32.Checksum(r[:12], castagnoli), castagnoli, b)
	binary.LittleEndian.PutUint32(r[12:16], sum)
	return r
}

// Write stores s: once it returns, a crash leaves s or a later state in the file
func (sf *SignFile) Write(s *SignState) error {
	b := s.Marshal()
	next := sf.seq + 1
	if sf.f == nil || int64(slotHeader+len(b)) > sf.slot {
		if err := sf.make(next, b); err != nil {
			return err
		}
		sf.seq = next
		return nil
	}
	if err := sf.writeSlot(record(next, b), int64(next%2)*sf.slot); err != nil {
		return err
	}
	// the sequence moves on only once the slot is synced: a failed write is retried in the same
	// slot, and the other keeps the last state stored
	sf.seq = next
	return nil
}

// writeSlot writes the record r at off and syncs it: directly in whole blocks, the rest of the last
// block zero, which is padding of the slot, or else with fdatasync
func (sf *SignFile) writeSlot(r []byte, off int64) error {
	if sf.w == nil {
		if _, err := sf.f.WriteAt(r, off); err != nil {
			return err
		}
		return fdatasync(sf.f)
	}
	buf := alignedBlocks((len(r) + directBlock - 1) / directBlock * directBlock)
	copy(buf, r)
	_, err := sf.w.WriteAt(buf, off)
	return err
}

// make writes the file anew with slots that hold b, and b at seq in its slot: to a temporary file,
// synced, renamed over the file, and the directory synced, so that after a crash the file has the
// state before or this one. It runs for the first state and when a state outgrows the slots.
func (sf *SignFile) make(seq uint64, b []byte) error {
	slot := int64(minSlot)
	for slot < 2*int64(slotHeader+len(b)) {
		slot *= 2
	}
	buf := make([]byte, 2*slot)
	copy(buf[int64(seq%2)*slot:], record(seq, b))

	tmp := sf.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(buf); err == nil {
		err = f.Sync()
	}
	if err == nil {
		err = os.Rename(tmp, sf.path)
	}
	if err == nil {
		err = syncDir(filepath.Dir(sf.path))
	}
	if err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	w, err := openDirect(sf.path)
	if err != nil {
		f.Close()
		return err
	}
	sf.Close()
	sf.f, sf.w, sf.slot = f, w, slot
	return nil
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if cerr := d.Close(); err == nil {
		err = cerr
	}
	return err
}

// Close closes the file
func (sf *SignFile) Close() error {
	if sf == nil || sf.f == nil {
		return nil
	}
	err := sf.f.Close()
	if sf.w != nil {
		if werr := sf.w.Close(); err == nil {
			err = werr
		}
	}
	sf.f, sf.w = nil, nil
	return err
}

// directBlock is the alignment and the unit of the direct writes: the logical block of the disks
// the node runs on is 512 bytes or 4 KiB
const directBlock = 4096

// alignedBlocks is a zero buffer of n bytes at an address aligned to directBlock
func alignedBlocks(n int) []byte {
	b := make([]byte, n+directBlock)
	off := (directBlock - int(uintptr(unsafe.Pointer(&b[0]))&(directBlock-1))) & (directBlock - 1)
	return b[off : off+n]
}
