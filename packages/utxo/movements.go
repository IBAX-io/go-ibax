/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package utxo lists the UTXO movements of an account: each transaction that made an output to
// it, or spent one of its outputs, in one ecosystem, as what that transaction did to the account.
package utxo

import (
	"bytes"
	"cmp"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/shopspring/decimal"
)

// The kinds of movement
const (
	KindGenesis  = "genesis"  // the first block made the output
	KindTransfer = "transfer" // a UTXO transfer the account sent or received
	KindMove     = "move"     // the account moved its tokens between its account and its UTXO
	KindFee      = "fee"      // the account paid in this ecosystem the fee of a transfer in another
	KindReward   = "reward"   // the account was paid the packaging or the taxes of another's transaction
)

// Page sizes
const (
	DefaultLimit = 25
	MaxLimit     = 100
)

// ErrInconsistent means the outputs name a transaction the blocks do not hold
var ErrInconsistent = errors.New("utxo movements: outputs and blocks disagree")

// Movement is what one transaction did to the account in the ecosystem; amounts are in base
// units, key IDs and block numbers decimal strings
type Movement struct {
	Hash  string `json:"hash"`
	Block string `json:"block"`
	// The time of the block, in RFC 3339
	Time string `json:"time"`
	Kind string `json:"kind"`
	// Where a move took the tokens: "utxo" or "account"
	To        string `json:"to,omitempty"`
	Sender    string `json:"sender"`
	Recipient string `json:"recipient"`
	// What was sent or moved; for a reward what the account was paid, for a fee 0
	Amount string `json:"amount"`
	// The packaging, taxes and combustion the transaction paid in this ecosystem
	Fee     string `json:"fee"`
	Comment string `json:"comment"`
	// What the account's UTXO gained, or lost when negative
	Net string `json:"net"`
}

// Page is the movements of whole blocks, the latest first; More says whether earlier blocks have more
type Page struct {
	Movements []Movement `json:"movements"`
	More      bool       `json:"more"`
}

// What the transaction asked, as its block holds it
type txFacts struct {
	Type      byte
	Sender    int64
	Ecosystem int64
	// A UTXO transfer's
	ToID    int64
	Value   string
	Comment string
	// A move's
	Target string
}

type candidate struct {
	Hash  []byte
	Block int64
}

// The transactions that made an output to the key or spent one of its, in the ecosystem; the
// spender's block is the one log_transactions has, since spending all may leave it no output
const candidates = `
SELECT hash, block FROM (
	SELECT output_tx_hash AS hash, block_id AS block FROM spent_info
	WHERE ecosystem = @ecosystem AND output_key_id = @key
	UNION
	SELECT si.input_tx_hash, lt.block FROM spent_info si JOIN log_transactions lt ON lt.hash = si.input_tx_hash
	WHERE si.ecosystem = @ecosystem AND si.output_key_id = @key AND si.input_tx_hash IS NOT NULL
) movements `

// Movements is the page of the key's movements in the ecosystem in blocks before the one given
// (0 for none), at least limit of them unless no more are left, and never part of a block
func Movements(key, ecosystem, before int64, limit int) (*Page, error) {
	if limit < 1 || limit > MaxLimit {
		return nil, fmt.Errorf("utxo movements: limit %d is not 1 to %d", limit, MaxLimit)
	}
	if before <= 0 {
		before = 1<<63 - 1
	}
	db := sqldb.GetDB(nil)
	params := map[string]any{"ecosystem": ecosystem, "key": key, "before": before}
	var found []candidate
	if err := db.Raw(candidates+`WHERE block < @before ORDER BY block DESC, hash LIMIT @limit`,
		withParam(params, "limit", limit)).Scan(&found).Error; err != nil {
		return nil, err
	}
	page := &Page{Movements: []Movement{}}
	if len(found) == limit {
		last := found[len(found)-1].Block
		var rest []candidate
		if err := db.Raw(candidates+`WHERE block = @last ORDER BY hash`, withParam(params, "last", last)).
			Scan(&rest).Error; err != nil {
			return nil, err
		}
		for _, c := range rest {
			if !slices.ContainsFunc(found, func(f candidate) bool { return bytes.Equal(f.Hash, c.Hash) }) {
				found = append(found, c)
			}
		}
		if err := db.Raw(`SELECT EXISTS (`+candidates+`WHERE block < @last)`, withParam(params, "last", last)).
			Row().Scan(&page.More); err != nil {
			return nil, err
		}
	}
	if len(found) == 0 {
		return page, nil
	}
	slices.SortFunc(found, func(a, b candidate) int {
		if a.Block != b.Block {
			return cmp.Compare(b.Block, a.Block)
		}
		return bytes.Compare(a.Hash, b.Hash)
	})
	hashes := make([][]byte, len(found))
	for i, c := range found {
		hashes[i] = c.Hash
	}
	var outputs, inputs []sqldb.SpentInfo
	if err := db.Where("ecosystem = ? AND output_tx_hash IN ?", ecosystem, hashes).Find(&outputs).Error; err != nil {
		return nil, err
	}
	if err := db.Where("ecosystem = ? AND output_key_id = ? AND input_tx_hash IN ?", ecosystem, key, hashes).
		Find(&inputs).Error; err != nil {
		return nil, err
	}
	blocks := map[int64]*block.Block{}
	for _, c := range found {
		b, ok := blocks[c.Block]
		if !ok {
			var err error
			if b, err = readBlock(c.Block); err != nil {
				return nil, err
			}
			blocks[c.Block] = b
		}
		facts, ok := factsOf(b, c.Hash)
		if !ok {
			return nil, fmt.Errorf("%w: transaction %x is not in block %d", ErrInconsistent, c.Hash, c.Block)
		}
		movement, err := classify(key, ecosystem, facts,
			filter(outputs, func(o sqldb.SpentInfo) bool { return bytes.Equal(o.OutputTxHash, c.Hash) }),
			filter(inputs, func(i sqldb.SpentInfo) bool { return bytes.Equal(i.InputTxHash, c.Hash) }))
		if err != nil {
			return nil, fmt.Errorf("%w: transaction %x: %w", ErrInconsistent, c.Hash, err)
		}
		movement.Hash = hex.EncodeToString(c.Hash)
		movement.Block = strconv.FormatInt(c.Block, 10)
		movement.Time = time.Unix(b.Header.Timestamp, 0).UTC().Format(time.RFC3339)
		page.Movements = append(page.Movements, movement)
	}
	return page, nil
}

func withParam(params map[string]any, name string, value any) map[string]any {
	with := map[string]any{name: value}
	for k, v := range params {
		with[k] = v
	}
	return with
}

func filter(rows []sqldb.SpentInfo, keep func(sqldb.SpentInfo) bool) []sqldb.SpentInfo {
	var kept []sqldb.SpentInfo
	for _, row := range rows {
		if keep(row) {
			kept = append(kept, row)
		}
	}
	return kept
}

func readBlock(id int64) (*block.Block, error) {
	record := &sqldb.BlockChain{}
	found, err := record.Get(id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%w: block %d is missing", ErrInconsistent, id)
	}
	return block.UnmarshallBlock(bytes.NewBuffer(record.Data), false)
}

func factsOf(b *block.Block, hash []byte) (txFacts, bool) {
	for _, tx := range b.Transactions {
		if !bytes.Equal(tx.Hash(), hash) {
			continue
		}
		facts := txFacts{Type: tx.Type(), Sender: tx.KeyID()}
		if smart, ok := tx.Inner.(*transaction.SmartTransactionParser); ok && smart.TxSmart != nil {
			facts.Ecosystem = smart.TxSmart.EcosystemID
			if utxo := smart.TxSmart.UTXO; utxo != nil {
				facts.ToID, facts.Value, facts.Comment = utxo.ToID, utxo.Value, utxo.Comment
			}
			if self := smart.TxSmart.TransferSelf; self != nil {
				facts.Value, facts.Target = self.Value, strings.ToLower(self.Target)
			}
		}
		return facts, true
	}
	return txFacts{}, false
}

func sum(rows []sqldb.SpentInfo, keep func(sqldb.SpentInfo) bool) (decimal.Decimal, error) {
	total := decimal.Zero
	for _, row := range rows {
		if !keep(row) {
			continue
		}
		value, err := decimal.NewFromString(row.OutputValue)
		if err != nil {
			return total, err
		}
		total = total.Add(value)
	}
	return total, nil
}

func isFee(output sqldb.SpentInfo) bool {
	return output.Type == consts.UTXO_Type_Packaging || output.Type == consts.UTXO_Type_Taxes ||
		output.Type == consts.UTXO_Type_Combustion
}

// What the transaction did to the key in the ecosystem, from what it asked, the outputs it made
// in the ecosystem and the key's outputs it spent there
func classify(key, ecosystem int64, facts txFacts, outputs, spent []sqldb.SpentInfo) (Movement, error) {
	toKey := func(o sqldb.SpentInfo) bool { return o.OutputKeyId == key }
	received, err := sum(outputs, toKey)
	if err != nil {
		return Movement{}, err
	}
	paid, err := sum(spent, func(sqldb.SpentInfo) bool { return true })
	if err != nil {
		return Movement{}, err
	}
	fee, err := sum(outputs, isFee)
	if err != nil {
		return Movement{}, err
	}
	m := Movement{
		Sender:    strconv.FormatInt(facts.Sender, 10),
		Recipient: strconv.FormatInt(key, 10),
		Amount:    "0",
		Fee:       fee.String(),
		Net:       received.Sub(paid).String(),
	}
	switch {
	case facts.Type == types.FirstBlockTxType:
		m.Kind, m.Sender, m.Amount = KindGenesis, "0", received.String()
	case facts.Type == types.TransferSelfTxType && facts.Sender == key:
		if facts.Target != "utxo" && facts.Target != "account" {
			return Movement{}, fmt.Errorf("a move to %q", facts.Target)
		}
		m.Kind, m.To, m.Amount = KindMove, facts.Target, facts.Value
	case facts.Type == types.UtxoTxType && facts.Ecosystem == ecosystem && (facts.Sender == key || facts.ToID == key):
		m.Kind, m.Recipient, m.Amount, m.Comment = KindTransfer, strconv.FormatInt(facts.ToID, 10), facts.Value, facts.Comment
	case facts.Type == types.UtxoTxType && facts.Sender == key:
		m.Kind, m.Recipient, m.Comment = KindFee, strconv.FormatInt(facts.ToID, 10), facts.Comment
	case facts.Type == types.UtxoTxType && slices.ContainsFunc(outputs, func(o sqldb.SpentInfo) bool { return toKey(o) && isFee(o) }):
		m.Kind, m.Amount = KindReward, received.String()
	default:
		return Movement{}, fmt.Errorf("a transaction of type %d made no movement of key %d", facts.Type, key)
	}
	return m, nil
}
