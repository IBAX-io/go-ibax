/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"errors"
	"math"
	"sync"

	"github.com/IBAX-io/go-ibax/packages/consts"
)

// windowMargin is how far, in seconds, the time of a check may be behind the time of the latest
// block for the window to answer it: a check further behind reads log_transactions
const windowMargin = 60

// WindowTx is a transaction of a committed block: its hash and the time it is signed at
type WindowTx struct {
	Hash   []byte
	Signed int64
}

// txWindow holds the hashes of the transactions of the committed blocks that a transaction can
// still be a duplicate of. A transaction is refused whose signed time is earlier than MaxTxBack
// before the time it is checked at, and a duplicate has the signed time of the transaction it
// repeats, so a committed transaction signed earlier than MaxTxBack before the latest block, less
// the margin, is let go. The window rolls forward with the blocks the node commits and is loaded
// from log_transactions, which keeps every transaction of the chain, at its first use.
type txWindow struct {
	mu     sync.RWMutex
	loaded bool
	// signed is the signed time of each hash, or an upper bound of it for those loaded
	signed map[string]int64
	// bySecond are the hashes of each signed second, from low on, which pruning lets go in order
	bySecond map[int64][]string
	low      int64
	// latest is the time of the latest committed block
	latest int64
	// floor is the earliest signed time from which the window holds every committed transaction
	floor int64
}

// TxWindow is the window of the hashes of the committed transactions the node checks duplicates in
var TxWindow = &txWindow{}

// LoadTxWindow loads the window from log_transactions and the blocks committed after the last
// executed one, at the start of the node
func LoadTxWindow() error {
	TxWindow.mu.Lock()
	defer TxWindow.mu.Unlock()
	return TxWindow.load()
}

// ResetTxWindow forgets the window, which the next use loads again: the state of the node changed
// otherwise than by its blocks (a join from a snapshot)
func ResetTxWindow() {
	TxWindow.mu.Lock()
	defer TxWindow.mu.Unlock()
	TxWindow.loaded = false
	TxWindow.signed, TxWindow.bySecond = nil, nil
}

// load reads the transactions of the blocks whose time is late enough that a transaction of them
// can be signed within the window: a block of time B takes transactions signed at most MaxTxForw
// after B, whose signed time stands at that bound until a block commits them again
func (w *txWindow) load() error {
	var latest []int64
	if err := DBConn.Raw(`SELECT "time" FROM "block_chain" ORDER BY "id" DESC LIMIT 1`).Scan(&latest).Error; err != nil {
		return err
	}
	w.signed, w.bySecond = make(map[string]int64), make(map[int64][]string)
	w.latest, w.floor, w.low = 0, math.MinInt64, math.MinInt64
	if len(latest) == 1 {
		w.latest = latest[0]
		w.floor = w.cutoff()
		w.low = w.floor
		var rows []struct {
			Hash []byte
			Time int64
		}
		if err := DBConn.Raw(`SELECT l."hash", b."time" FROM "block_chain" b JOIN "log_transactions" l ON l."block" = b."id"
			WHERE b."id" >= (SELECT coalesce(min("id"), 0) FROM "block_chain" WHERE "time" >= ?)`,
			w.floor-consts.MaxTxForw).Scan(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			w.put(string(r.Hash), r.Time+consts.MaxTxForw)
		}
		if err := w.loadUnexecuted(); err != nil {
			return err
		}
	}
	w.loaded = true
	return nil
}

// ParseWindowTxs is the transactions of the data of a block as the window takes them; package
// block sets it, as the parsing of a block is its own
var ParseWindowTxs func(data []byte) ([]WindowTx, error)

// loadUnexecuted adds the transactions of the blocks of an ordered chain committed after the last
// block the node executed: their transactions are spent though no log of them is written yet
func (w *txWindow) loadUnexecuted() error {
	var rows []struct {
		Data []byte
	}
	if err := DBConn.Raw(`SELECT "data" FROM "block_chain" WHERE "id" > (SELECT coalesce(max("block_id"), 0) FROM "info_block")
		ORDER BY "id"`).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) > 0 && ParseWindowTxs == nil {
		return errors.New("blocks committed after the last executed block, and no parser of their transactions")
	}
	for _, r := range rows {
		txs, err := ParseWindowTxs(r.Data)
		if err != nil {
			return err
		}
		for _, t := range txs {
			w.put(string(t.Hash), t.Signed)
		}
	}
	return nil
}

// cutoff is the earliest signed time the window keeps
func (w *txWindow) cutoff() int64 {
	return w.latest - consts.MaxTxBack - windowMargin
}

func (w *txWindow) put(hash string, signed int64) {
	if prev, ok := w.signed[hash]; ok && prev >= signed {
		return
	}
	w.signed[hash] = signed
	w.bySecond[signed] = append(w.bySecond[signed], hash)
}

// ensure loads the window unless it is
func (w *txWindow) ensure() error {
	w.mu.RLock()
	loaded := w.loaded
	w.mu.RUnlock()
	if loaded {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.loaded {
		return nil
	}
	return w.load()
}

// Add adds the transactions of the block of time blockTime the node committed, and lets go those
// signed too early to be repeated
func (w *txWindow) Add(blockTime int64, txs []WindowTx) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.loaded {
		// the block is committed: loading reads it
		return w.load()
	}
	for _, t := range txs {
		w.put(string(t.Hash), t.Signed)
	}
	if blockTime > w.latest {
		w.latest = blockTime
	}
	cut := w.cutoff()
	for ; w.low < cut && len(w.signed) > 0; w.low++ {
		if w.low == math.MinInt64 {
			w.low = cut
			break
		}
		for _, h := range w.bySecond[w.low] {
			if s, ok := w.signed[h]; ok && s < cut {
				delete(w.signed, h)
			}
		}
		delete(w.bySecond, w.low)
	}
	if w.low < cut {
		w.low = cut
	}
	if cut > w.floor {
		w.floor = cut
	}
	return nil
}

// Remove lets go the hashes of the transactions of a block the node rolled back
func (w *txWindow) Remove(hashes [][]byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.loaded {
		return
	}
	for _, h := range hashes {
		delete(w.signed, string(h))
	}
}

// Known are the hashes of hashes in a committed block, for a check at checkTime: of the window,
// or of log_transactions for a check the window started after
func (w *txWindow) Known(hashes [][]byte, checkTime int64) (map[string]bool, error) {
	if len(hashes) == 0 {
		return map[string]bool{}, nil
	}
	if err := w.ensure(); err != nil {
		return nil, err
	}
	w.mu.RLock()
	if checkTime-consts.MaxTxBack < w.floor {
		w.mu.RUnlock()
		return KnownLogTransactions(nil, hashes)
	}
	known := make(map[string]bool)
	for _, h := range hashes {
		if _, ok := w.signed[string(h)]; ok {
			known[string(h)] = true
		}
	}
	w.mu.RUnlock()
	return known, nil
}

// Has tells a committed block holds the transaction of the hash, of those the window keeps
func (w *txWindow) Has(hash []byte) (bool, error) {
	if err := w.ensure(); err != nil {
		return false, err
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	_, ok := w.signed[string(hash)]
	return ok, nil
}

// Len is the hashes the window holds
func (w *txWindow) Len() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.signed)
}
