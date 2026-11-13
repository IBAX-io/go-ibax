/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

// The comparison of the statements of the contract functions with those of b7_oldpath_test.go:
// both run the same calls on the same state, in a view run in parallel, in a view run in order,
// in a view whose tables are all in PostgreSQL, and in a transaction without a view. What a run
// records is all a contract, a block and the hashes of a block take of the statements: the values
// the functions return, the values they change of their arguments, the statements and the values
// of their placeholders, the rollback records, the credits of the fees, the counts of the
// statements and the reads and writes of the view.

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/pbgo"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
)

const (
	b7hKeys  = 24
	b7hBase  = int64(-8810000)
	b7hBlock = int64(5100)
	// b7hTyped is the custom table of the comparisons, of the types the builder treats apart
	b7hTyped = "b7_typed"
)

func b7hAccount(i int) string { return fmt.Sprintf("B8-%021d", i) }

// b7Impl are the contract functions a run calls: those of the package or the old ones
type b7Impl struct {
	name      string
	selectFn  func(sc *SmartContract, tbl string, cols any, id int64, order any, offset, limit int64, where *types.Map, query any, group string, all bool) (int64, []any, error)
	updateExt func(sc *SmartContract, tbl string, where, values *types.Map) (int64, error)
	insert    func(sc *SmartContract, tbl string, values *types.Map) (int64, int64, error)
	keyAmount func(sc *SmartContract, sign string, sum decimal.Decimal, eco, id int64, where *types.Map) (decimal.Decimal, error)
	payTaxes  func(sc *SmartContract, pay *PaymentInfo, sum decimal.Decimal, t GasScenesType, comment string, status pbgo.TxInvokeStatusCode) error
	writeRows func(sc *SmartContract, fields []string, ivalues []any, table string, where *types.Map, generalRollback, exists bool) (int64, string, *sqldb.OneRow, error)
}

var (
	b7New = b7Impl{name: "new", selectFn: DBSelect, updateExt: DBUpdateExt, insert: DBInsert,
		keyAmount: func(sc *SmartContract, sign string, sum decimal.Decimal, eco, id int64, where *types.Map) (decimal.Decimal, error) {
			return sc.updateKeyAmount(sign, sum, eco, id, where)
		},
		payTaxes: func(sc *SmartContract, pay *PaymentInfo, sum decimal.Decimal, t GasScenesType, comment string, status pbgo.TxInvokeStatusCode) error {
			return sc.payTaxes(pay, sum, t, comment, status)
		},
		writeRows: func(sc *SmartContract, fields []string, ivalues []any, table string, where *types.Map, generalRollback, exists bool) (int64, string, *sqldb.OneRow, error) {
			return sc.writeRows(fields, ivalues, table, where, generalRollback, exists)
		}}
	b7Old = b7Impl{name: "old", selectFn: b7OldDBSelect, updateExt: b7OldDBUpdateExt, insert: b7OldDBInsert,
		keyAmount: b7OldUpdateKeyAmount, payTaxes: b7OldPayTaxes, writeRows: b7OldWriteRows}
)

// b7Call is a run of a scenario: the functions it calls and what it records
type b7Call struct {
	impl b7Impl
	sc   *SmartContract
	log  strings.Builder
	// failed tells a call failed: a view that has to run in order runs nothing more
	failed bool
}

func b7Fmt(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case string:
		return fmt.Sprintf("%q", x)
	case []byte:
		return fmt.Sprintf("bytes(%x)", x)
	case *types.Map:
		if x == nil {
			return "map(nil)"
		}
		parts := make([]string, 0, x.Size())
		for _, k := range x.Keys() {
			e, _ := x.Get(k)
			parts = append(parts, fmt.Sprintf("%q:%s", k, b7Fmt(e)))
		}
		return "map{" + strings.Join(parts, ",") + "}"
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = b7Fmt(e)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case []string:
		return fmt.Sprintf("%q", x)
	case decimal.Decimal:
		return "dec(" + x.String() + ")"
	case error:
		return "error(" + x.Error() + ")"
	}
	return fmt.Sprintf("%T(%v)", v, v)
}

func (c *b7Call) rec(name string, kv ...any) {
	fmt.Fprintf(&c.log, "%s", name)
	for _, v := range kv {
		fmt.Fprintf(&c.log, " %s", b7Fmt(v))
	}
	c.log.WriteByte('\n')
}

func (c *b7Call) done(err error) {
	if err != nil {
		c.failed = true
	}
}

func (c *b7Call) Select(tbl string, cols any, id int64, order any, offset, limit int64, where *types.Map, query any, group string, all bool) []any {
	if c.failed {
		return nil
	}
	cost, rows, err := c.impl.selectFn(c.sc, tbl, cols, id, order, offset, limit, where, query, group, all)
	c.rec("select", tbl, cols, id, order, offset, limit, where, query, group, all, "->", cost, rows, err)
	c.done(err)
	return rows
}

func (c *b7Call) UpdateExt(tbl string, where, values *types.Map) {
	if c.failed {
		return
	}
	cost, err := c.impl.updateExt(c.sc, tbl, where, values)
	c.rec("update", tbl, where, values, "->", cost, err)
	c.done(err)
}

func (c *b7Call) Insert(tbl string, values *types.Map) {
	if c.failed {
		return
	}
	cost, id, err := c.impl.insert(c.sc, tbl, values)
	c.rec("insert", tbl, values, "->", cost, id, err)
	c.done(err)
}

func (c *b7Call) KeyAmount(sign string, sum decimal.Decimal, eco, id int64, where *types.Map) {
	if c.failed {
		return
	}
	balance, err := c.impl.keyAmount(c.sc, sign, sum, eco, id, where)
	c.rec("keyAmount", sign, sum, eco, id, where, "->", balance, err)
	c.done(err)
}

func (c *b7Call) PayTaxes(pay *PaymentInfo, sum decimal.Decimal, t GasScenesType, comment string) {
	if c.failed {
		return
	}
	err := c.impl.payTaxes(c.sc, pay, sum, t, comment, pbgo.TxInvokeStatusCode_SUCCESS)
	c.rec("payTaxes", sum, int64(t), comment, "->", err)
	c.done(err)
}

func (c *b7Call) WriteRows(fields []string, ivalues []any, table string, where *types.Map, exists bool) {
	if c.failed {
		return
	}
	cost, id, rows, err := c.impl.writeRows(c.sc, fields, ivalues, table, where, true, exists)
	var list []map[string]string
	if rows != nil {
		list = rows.List
	}
	c.rec("writeRows", fields, ivalues, table, where, exists, "->", cost, id, fmt.Sprintf("%v", list), err)
	c.done(err)
}

// b7m is a map of the pairs in their order
func b7m(kv ...any) *types.Map {
	r := types.NewMap()
	for j := 0; j < len(kv); j += 2 {
		r.Set(kv[j].(string), kv[j+1])
	}
	return r
}

// b7Scenario is a sequence of calls of one transaction
type b7Scenario struct {
	name string
	// full runs the scenario with the full access of a system contract
	full bool
	run  func(c *b7Call)
}

func b7hTransfer(c *b7Call, s int) {
	sender, recipient := b7hAccount(s), b7hAccount(s+1)
	amount := decimal.NewFromInt(1000)
	c.Select("@1ecosystems", nil, 0, nil, 0, 1, b7m("id", int64(1)), nil, "", false)
	c.Select("@1keys", nil, 0, nil, 0, 1, b7m("ecosystem", int64(1), "account", sender, "deleted", int64(0)), nil, "", false)
	c.UpdateExt("@1keys", b7m("ecosystem", int64(1), "account", sender, "deleted", int64(0)), b7m("-amount", amount))
	c.Select("@1keys", nil, 0, nil, 0, 1, b7m("ecosystem", int64(1), "account", recipient), nil, "", false)
	c.UpdateExt("@1keys", b7m("ecosystem", int64(1), "account", recipient, "deleted", int64(0)), b7m("+amount", amount))
	sb := c.Select("@1keys", []any{"amount"}, 0, nil, 0, 1, b7m("account", sender, "ecosystem", int64(1)), nil, "", false)
	rb := c.Select("@1keys", []any{"amount"}, 0, nil, 0, 1, b7m("account", recipient, "ecosystem", int64(1)), nil, "", false)
	bal := func(r []any) any {
		if len(r) == 0 {
			return "0"
		}
		v, _ := r[0].(*types.Map).Get("amount")
		return v
	}
	c.Insert("@1history", b7m("sender_id", b7hBase-int64(s), "recipient_id", b7hBase-int64(s+1),
		"sender_balance", bal(sb), "recipient_balance", bal(rb), "amount", amount, "comment", "",
		"block_id", b7hBlock, "txhash", c.sc.Hash, "ecosystem", int64(1), "type", int64(3), "created_at", int64(1700000000)))
	pay := benchPayment()
	pay.FromID, pay.ToID, pay.TaxesID = b7hBase-int64(s), 5555, 0
	c.PayTaxes(pay, decimal.NewFromInt(970000), GasScenesType_Reward, "taxes for execution of @1TokensTransfer contract")
	c.PayTaxes(pay, decimal.NewFromInt(30000), GasScenesType_Taxes, "taxes for execution of @1TokensTransfer contract")
}

func b7hIDs(n int) []any {
	ids := make([]any, n)
	for i := range ids {
		ids[i] = b7hBase - int64(i)
	}
	return ids
}

// b7Scenarios are the shapes of the comparisons: those of a transfer, and those of the forms of
// the fields, the values and the conditions the query builder writes apart. A scenario ends at
// its first failing call, as a contract does.
func b7Scenarios() []b7Scenario {
	keys := func(name string, full bool, run func(c *b7Call)) b7Scenario {
		return b7Scenario{name: name, full: full, run: run}
	}
	sel := func(c *b7Call, cols any, where *types.Map) {
		c.Select("@1keys", cols, 0, nil, 0, 5, where, nil, "", false)
	}
	id := b7hBase - 12
	list := []b7Scenario{
		keys("transfer", false, func(c *b7Call) { b7hTransfer(c, 2) }),
		keys("transfer-repeat", false, func(c *b7Call) {
			b7hTransfer(c, 4)
			b7hTransfer(c, 6)
		}),
		keys("select-forms", false, func(c *b7Call) {
			c.Select("@1keys", nil, b7hBase-3, nil, 0, 0, nil, nil, "", false)
			c.Select("@1keys", "amount,account", 0, b7m("id", int64(-1)), 1, 3, b7m("ecosystem", int64(1), "id", b7m("$lte", b7hBase)), nil, "", false)
			c.Select("@1keys", []any{"id", "amount"}, 0, []any{b7m("amount", int64(1)), "id"}, 0, 10,
				b7m("$or", []any{b7m("account", b7hAccount(1)), b7m("account", b7hAccount(2), "deleted", int64(0))}, "ecosystem", int64(1)), nil, "", false)
			sel(c, []any{"account"}, b7m("$and", []any{b7m("id", b7m("$gt", b7hBase-6)), b7m("id", b7m("$neq", b7hBase-2))}, "ecosystem", int64(1)))
			sel(c, []any{"account"}, b7m("account", b7m("$begin", "B8-0000"), "ecosystem", int64(1)))
			sel(c, []any{"account"}, b7m("account", b7m("$like", "00000000000000000000"), "id", b7m("$lt", b7hBase), "ecosystem", int64(1)))
			sel(c, []any{"account"}, b7m("id", b7m("$nin", b7hIDs(3)), "account", b7m("$ibegin", "b8-"), "ecosystem", int64(1)))
			sel(c, []any{"account"}, b7m("account", b7m("$end", "05"), "id", b7m("$gte", b7hBase-9), "ecosystem", int64(1)))
			sel(c, []any{"account"}, b7m("account", b7m("$ilike", "000000005"), "ecosystem", int64(1)))
			sel(c, []any{"account"}, b7m("account", b7m("$iend", "05"), "ecosystem", int64(1)))
			sel(c, []any{"account"}, b7m("conditions", "$isnull", "account", b7hAccount(5), "ecosystem", int64(1)))
			sel(c, []any{"account"}, b7m("pub", []byte{1}, "account", b7hAccount(6), "ecosystem", int64(1)))
			sel(c, []any{"account"}, b7m("id", []any{b7hBase - 7, b7m("$gte", b7hBase-8)}, "ecosystem", int64(1)))
			sel(c, []any{"account"}, b7m("id", b7m("$in", b7hIDs(4)), "ecosystem", int64(1), "Account", b7m("$neq", "x")))
			sel(c, []any{"amount"}, b7m("id", b7m("$eq", b7hBase-9), "ecosystem", "1"))
			sel(c, []any{"amount"}, b7m("id", decimal.NewFromInt(b7hBase-11), "ecosystem", int64(1), "deleted", 0.0))
			sel(c, []any{"amount"}, b7m("id", b7m("$in", []any{}), "ecosystem", int64(1)))
			c.Select("@1ecosystems", []any{"info->name", "digits"}, 0, nil, 0, 1, b7m("id", int64(1)), nil, "", false)
			c.Select("@1ecosystems", "id,info->a->b", 0, nil, 0, 1, b7m("info->name", "x"), nil, "", false)
			c.Select("@1keys", []any{"amount"}, 0, nil, 0, 1, b7m("id", b7hBase-int64(10), "ecosystem", int64(1)), nil, "", true)
			c.Select("@1keys", "*", 0, nil, 0, -1, b7m("id", b7hBase-int64(10), "ecosystem", int64(1)), nil, "", false)
			c.Select("@1keys", []any{"ecosystem"}, 0, "ecosystem", 0, 3, b7m("id", b7m("$lte", b7hBase), "ecosystem", int64(1)), "count(*) as n, ecosystem", "ecosystem", false)
			c.Select("@1keys", []any{"amount"}, 0, nil, 0, 1, nil, nil, "", false)
		}),
		keys("select-in-text", false, func(c *b7Call) {
			sel(c, []any{"account"}, b7m("id", b7m("$in", b7hAccount(3)), "ecosystem", int64(1)))
		}),
		keys("select-bool", false, func(c *b7Call) {
			sel(c, []any{"amount"}, b7m("id", b7hBase-11, "deleted", false))
		}),
		keys("update-sign", false, func(c *b7Call) {
			c.UpdateExt("@1keys", b7m("id", id), b7m("+amount", decimal.RequireFromString("12345678901234567890.5")))
			c.UpdateExt("@1keys", b7m("id", id, "ecosystem", int64(1)), b7m("-amount", int64(7)))
			c.UpdateExt("@1keys", b7m("id", b7m("$in", b7hIDs(1))), b7m("+amount", decimal.NewFromInt(1)))
			for n := 2; n <= 8; n++ {
				c.UpdateExt("@1keys", b7m("id", b7m("$in", b7hIDs(n)), "ecosystem", int64(1)), b7m("-amount", decimal.NewFromInt(int64(n))))
			}
			c.UpdateExt("@1keys", b7m("account", b7hAccount(13), "deleted", int64(0)), b7m("-amount", "0.5"))
			c.KeyAmount("-", decimal.NewFromInt(5), 1, id, b7m("id", id, "ecosystem", int64(1)))
			c.KeyAmount("+", decimal.RequireFromString("0.25"), 1, id, b7m("id", id, "ecosystem", int64(1)))
			c.KeyAmount("-", decimal.NewFromInt(5), 1, id, b7m("id", id, "ecosystem", int64(1)))
			c.Select("@1keys", nil, id, nil, 0, 0, nil, nil, "", false)
		}),
		keys("update-plain", true, func(c *b7Call) {
			c.UpdateExt("@1keys", b7m("account", b7hAccount(12)), b7m("maxpay", "1000", "conditions", "true"))
			c.UpdateExt("@1keys", b7m("id", id), b7m("pub", "0a0b0c", "conditions", `ContractAccess("@1X")`))
			c.UpdateExt("@1keys", b7m("id", id), b7m("pub", "", "multi", int64(1)))
			c.UpdateExt("@1keys", b7m("id", id), b7m("pub", []byte("raw<&>"), "blocked", int64(0)))
			c.UpdateExt("@1keys", b7m("id", id), b7m("maxpay", 2.5, "multi", int64(2)))
			c.UpdateExt("@1keys", b7m("id", id, "ecosystem", int64(1)), b7m("ecosystem", int64(1), "multi", int64(3)))
			c.Select("@1keys", nil, id, nil, 0, 0, nil, nil, "", false)
			c.WriteRows([]string{"Multi", "maxpay"}, []any{int64(4), "9"}, "1_keys", b7m("id", id), true)
		}),
		keys("update-null", true, func(c *b7Call) {
			c.UpdateExt("@1keys", b7m("id", id), b7m("conditions", "NULL"))
		}),
		keys("update-float", true, func(c *b7Call) {
			c.UpdateExt("@1keys", b7m("id", id), b7m("multi", 2.5))
		}),
		keys("update-missing", false, func(c *b7Call) {
			c.UpdateExt("@1keys", b7m("id", int64(-1)), b7m("+amount", int64(1)))
		}),
		keys("update-denied", false, func(c *b7Call) {
			c.UpdateExt("@1keys", b7m("id", id), b7m("maxpay", "1"))
		}),
		keys("insert-forms", false, func(c *b7Call) {
			c.Insert("@1history", b7m("sender_id", b7hBase, "recipient_id", int64(0), "amount", "5", "comment", "quote \" <tag> & \u2028 \xff",
				"block_id", b7hBlock, "txhash", []byte{0, 1, 2}, "type", int64(1), "created_at", int64(1)))
			c.Insert("@1history", b7m("sender_id", b7hBase, "amount", decimal.RequireFromString("-0.000001"), "comment", "c",
				"block_id", b7hBlock, "txhash", "", "type", int64(2), "created_at", int64(2), "value_detail", `{"a": "b"}`))
			c.Insert("@1history", b7m("sender_id", b7hBase, "amount", "1", "comment", "x", "block_id", b7hBlock,
				"txhash", "abcdef", "ecosystem", int64(1), "type", int64(2), "created_at", int64(3), "value_detail->k", "v"))
			c.Insert("@1history", b7m("sender_id", b7hBase, "amount", "1", "comment", "", "block_id", b7hBlock, "type", int64(2), "created_at", int64(3)))
			c.Insert("@1history", b7m("sender_id", b7hBase, "amount", "1", "comment", "", "block_id", b7hBlock, "type", int64(2), "created_at", int64(3),
				"value_detail", "NULL"))
			c.WriteRows([]string{"Sender_ID", "amount", "type", "comment"}, []any{b7hBase, "2", int64(4), "y"}, "1_history", nil, false)
		}),
		keys("insert-map", false, func(c *b7Call) {
			c.Insert("@1history", b7m("sender_id", b7hBase, "amount", "1", "comment", "", "block_id", b7hBlock, "type", int64(2), "created_at", int64(3),
				"value_detail", b7m("a", "b")))
		}),
		keys("insert-key", true, func(c *b7Call) {
			c.Insert("@1keys", b7m("id", b7hBase-30, "account", b7hAccount(30), "pub", "0102", "amount", int64(5)))
		}),
		keys("insert-null", false, func(c *b7Call) {
			c.Insert("@1history", b7m("sender_id", b7hBase, "amount", "1", "comment", "NULL", "block_id", b7hBlock, "type", int64(2), "created_at", int64(3)))
		}),
		keys("fees", false, func(c *b7Call) {
			pay := benchPayment()
			pay.FromID, pay.ToID, pay.TaxesID = b7hBase-14, 5555, b7hBase-15
			c.PayTaxes(pay, decimal.NewFromInt(970000), GasScenesType_Reward, "reward")
			c.PayTaxes(pay, decimal.NewFromInt(30000), GasScenesType_Taxes, "taxes")
			c.PayTaxes(pay, decimal.NewFromInt(100), GasScenesType_Combustion, "combustion")
			c.PayTaxes(pay, decimal.Zero, GasScenesType_Taxes, "zero")
			pay.ToID = pay.FromID
			c.PayTaxes(pay, decimal.NewFromInt(1), GasScenesType_Direct, "self")
		}),
		keys("fees-missing", false, func(c *b7Call) {
			pay := benchPayment()
			pay.FromID, pay.ToID = int64(-1), 5555
			c.PayTaxes(pay, decimal.NewFromInt(1), GasScenesType_Reward, "missing")
		}),
	}
	return list
}

// b7TypedScenarios are the scenarios of the custom table b7hTyped: jsonb, bytea, date, time and
// timestamp columns, permissions of the table and the columns
func b7TypedScenarios() []b7Scenario {
	tbl := "@1" + b7hTyped
	one := func(name string, run func(c *b7Call)) b7Scenario { return b7Scenario{name: "typed-" + name, run: run} }
	return []b7Scenario{
		one("insert", func(c *b7Call) {
			c.Insert(tbl, b7m("id", int64(-11), "name", "a", "data", `{"x": 1}`, "bin", "00ff", "at", "2020-01-02 03:04:05", "day", "2020-01-02",
				"tm", "10:11:12", "amount", decimal.RequireFromString("1.50")))
			c.Insert(tbl, b7m("name", "b", "bin", "", "timestamp at", "1700000000", "day", "NULL"))
			c.Insert(tbl, b7m("name", "c", "at", "timestamp 2021-02-03 04:05:06", "tm", "NULL", "amount", 3.25, "bin", []byte{9}))
			c.Insert(tbl, b7m("name", "e", "bin", "zz", "secret", "s"))
			c.Select(tbl, []any{"id", "name", "data", "bin", "at", "day", "tm", "amount", "flag", "secret"}, 0, b7m("id", int64(1)), 0, 10,
				b7m("id", b7m("$lte", int64(-1))), nil, "", false)
		}),
		one("insert-json", func(c *b7Call) {
			c.Insert(tbl, b7m("name", "j", "data->k", "v", "data->j", "<w>"))
		}),
		one("insert-bool", func(c *b7Call) { c.Insert(tbl, b7m("name", "f", "flag", true)) }),
		one("insert-day", func(c *b7Call) { c.Insert(tbl, b7m("name", "d", "day", "today")) }),
		one("insert-unix", func(c *b7Call) { c.Insert(tbl, b7m("name", "u", "timestamp at", "now")) }),
		one("insert-ts", func(c *b7Call) { c.Insert(tbl, b7m("name", "t", "at", "timestamp 2021-13-01")) }),
		one("insert-unique", func(c *b7Call) { c.Insert(tbl, b7m("name", "r1")) }),
		one("update-json", func(c *b7Call) {
			c.UpdateExt(tbl, b7m("id", int64(-1)), b7m("data->a", "1", "data->b", "<2>"))
		}),
		one("update-json-null", func(c *b7Call) { c.UpdateExt(tbl, b7m("id", int64(-2)), b7m("data->a", "1")) }),
		one("update-json-empty", func(c *b7Call) { c.UpdateExt(tbl, b7m("id", int64(-3)), b7m("data->a", "1", "amount", int64(1))) }),
		one("update-forms", func(c *b7Call) {
			c.UpdateExt(tbl, b7m("name", "r1"), b7m("bin", "0102", "at", "timestamp 2022-01-01 00:00:00"))
			c.UpdateExt(tbl, b7m("name", "r1"), b7m("timestamp at", "1600000000.5"))
			c.UpdateExt(tbl, b7m("name", "r1"), b7m("bin", "", "day", "2023-03-03", "tm", "01:02"))
			c.UpdateExt(tbl, b7m("name", "r2"), b7m("bin", []byte("<>"), "day", "NULL", "tm", "NULL"))
			c.UpdateExt(tbl, b7m("name", b7m("$in", []any{"r1", "r2"})), b7m("amount", decimal.NewFromInt(9)))
			c.UpdateExt(tbl, b7m("at", b7m("$lt", "2030-01-01")), b7m("+amount", int64(1)))
			c.UpdateExt(tbl, b7m("day", "2023-03-03", "tm", b7m("$gt", "00:00")), b7m("-amount", int64(1)))
			c.UpdateExt(tbl, b7m("id", int64(-3)), b7m("secret", "s", "name", "r3"))
			c.UpdateExt(tbl, b7m("id", int64(-3)), b7m("secret", "<s&\u2028\x01>"))
			c.UpdateExt(tbl, b7m("id", int64(-3)), b7m("secret", "z"))
			c.Select(tbl, []any{"id", "name", "data", "bin", "at", "day", "tm", "amount", "flag", "secret", "data->a"}, 0, b7m("id", int64(1)), 0, 10,
				b7m("id", b7m("$lte", int64(-1))), nil, "", false)
			c.Select(tbl, "id,bin", 0, nil, 0, 10, b7m("bin", b7m("$eq", "\\x01")), nil, "", false)
		}),
		one("update-day", func(c *b7Call) { c.UpdateExt(tbl, b7m("day", "yesterday"), b7m("+amount", int64(1))) }),
		one("update-day-in", func(c *b7Call) {
			c.UpdateExt(tbl, b7m("day", b7m("$in", []any{"2020-01-01", "tomorrow"})), b7m("+amount", int64(1)))
		}),
		one("update-unix", func(c *b7Call) { c.UpdateExt(tbl, b7m("id", int64(-1)), b7m("timestamp at", "x")) }),
		one("update-ts", func(c *b7Call) { c.UpdateExt(tbl, b7m("id", int64(-1)), b7m("at", "timestamp now")) }),
		one("update-twice", func(c *b7Call) {
			c.UpdateExt(tbl, b7m("id", int64(-1)), b7m("at", "timestamp 2022-01-01 00:00:00", "timestamp at", "1"))
		}),
		one("update-denied", func(c *b7Call) { c.UpdateExt(tbl, b7m("id", int64(-2)), b7m("locked", "x")) }),
		one("update-now", func(c *b7Call) {
			c.UpdateExt(tbl, b7m("name", b7m("$eq", "x"), "$or", []any{b7m("name", "now()")}), b7m("amount", int64(1)))
		}),
		one("select-now", func(c *b7Call) { c.Select(tbl, "current_date", 0, nil, 0, 1, nil, nil, "", false) }),
		one("select-hidden", func(c *b7Call) { c.Select(tbl, "secret", 0, nil, 0, 1, nil, nil, "", false) }),
	}
}

// b7Trace records the statements of a run
type b7Trace struct {
	b       strings.Builder
	pgClass int
	texts   map[string]int
}

func (tr *b7Trace) add(query string, args []any) {
	if strings.Contains(query, "pg_class t, pg_class i") {
		tr.pgClass++
		return
	}
	tr.texts[query]++
	fmt.Fprintf(&tr.b, "stmt %s", query)
	for _, a := range args {
		fmt.Fprintf(&tr.b, " %s", b7Fmt(a))
	}
	tr.b.WriteByte('\n')
}

// b7Env is the database of the comparisons: the keys and the custom table committed
type b7Env struct {
	t    testing.TB
	conn *sql.Conn
	// shapes are the texts of the statements the runs made, and how many times
	shapes map[string]int
}

func b7Open(t testing.TB) *b7Env {
	dsn := os.Getenv("IBAX_TEST_DSN")
	if dsn == "" {
		t.Skip("IBAX_TEST_DSN names no database of a node")
	}
	pc, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	saved := sqldb.DBConn
	require.NoError(t, sqldb.GormInit(conf.DBConfig{Host: pc.Host, Port: int(pc.Port), User: pc.User, Name: pc.Database,
		Password: pc.Password, LockTimeout: 5000, IdleInTxTimeout: 50000, MaxIdleConns: 4, MaxOpenConns: 8}))
	InitVM()
	crypto.InitHashAlgo(crypto.HashAlgo_SHA256.String())
	seed := func(stmts ...string) {
		tr, err := sqldb.StartTransaction()
		require.NoError(t, err)
		for _, s := range stmts {
			require.NoError(t, tr.ExecSql(s), s)
		}
		require.NoError(t, tr.Commit())
	}
	drop := []string{
		fmt.Sprintf(`DELETE FROM "1_keys" WHERE id <= %d AND id > %d`, b7hBase, b7hBase-b7hKeys-10),
		`DROP TABLE IF EXISTS "1_` + b7hTyped + `"`,
		`DELETE FROM "1_tables" WHERE name = '` + b7hTyped + `' AND ecosystem = 1`,
	}
	stmts := append([]string{}, drop...)
	for i := 0; i < b7hKeys; i++ {
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO "1_keys" (id, pub, amount, ecosystem, account) VALUES (%d, '\x01', 1000000000000, 1, '%s')`,
			b7hBase-int64(i), b7hAccount(i)))
	}
	stmts = append(stmts,
		`CREATE TABLE "1_`+b7hTyped+`" (id bigint NOT NULL PRIMARY KEY, name varchar(100) NOT NULL DEFAULT '', data jsonb, bin bytea,
			at timestamp, day date, tm time, amount numeric(30,2) NOT NULL DEFAULT 0, flag boolean, secret text, locked text,
			ecosystem bigint NOT NULL DEFAULT 1)`,
		`CREATE UNIQUE INDEX "1_`+b7hTyped+`_name" ON "1_`+b7hTyped+`" (name)`,
		`INSERT INTO "1_`+b7hTyped+`" (id, name, data, bin, at, day, tm, amount, flag) VALUES
			(-1, 'r1', '{"a": "0"}', '\x01', '2020-01-01 00:00:00', '2020-01-01', '01:00', 1, true),
			(-2, 'r2', NULL, NULL, NULL, NULL, NULL, 2, NULL),
			(-3, 'r3', '{}', '', '2020-01-01 00:00:00.5', '2020-01-01', '01:00:00.25', 3, false)`,
		`INSERT INTO "1_tables" (id, name, permissions, columns, conditions, app_id, ecosystem) VALUES
			(next_id('1_tables'), '`+b7hTyped+`', '{"read": "true", "insert": "true", "update": "true", "new_column": "true"}',
			'{"name": "true", "data": "true", "bin": "true", "at": "true", "day": "true", "tm": "true", "amount": "true",
			"flag": "true", "secret": "{\"read\": \"false\", \"update\": \"true\"}", "locked": "ContractAccess(\"@1Nobody\")"}', 'true', 1, 1)`,
	)
	seed(stmts...)
	tr, err := sqldb.StartTransaction()
	require.NoError(t, err)
	require.NoError(t, syspar.SysTableColType(tr))
	require.NoError(t, tr.Rollback())
	conn, err := sqldb.PinConn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.Close()
		seed(drop...)
		sqldb.GormClose()
		sqldb.DBConn = saved
	})
	return &b7Env{t: t, conn: conn, shapes: map[string]int{}}
}

func b7hSC(tr *sqldb.DbTransaction, i int) *SmartContract {
	return &SmartContract{
		DbTransaction: tr,
		VM:            script.GetVM(),
		Key:           &sqldb.Key{},
		Rollback:      true,
		Hash:          []byte(fmt.Sprintf("b7-harness-hash-%016d", i)),
		Timestamp:     1700000000,
		BlockHeader:   &types.BlockHeader{BlockId: b7hBlock},
		TxContract:    &Contract{Name: "@1TokensTransfer", StackCont: []any{"@1TokensSend", "@1TokensTransfer"}},
		TxSmart:       &types.SmartTransaction{Header: &types.Header{EcosystemID: 1, KeyID: b7hBase}},
	}
}

// b7Modes are the ways a run executes its statements
var b7Modes = []string{"parallel", "ordered", "fallback", "pg"}

// run runs a scenario in a mode with the functions of impl on a state of its own, and gives what
// it recorded; nothing it writes stays
func (e *b7Env) run(s b7Scenario, mode string, impl b7Impl) string {
	t := e.t
	tr, err := sqldb.StartTransaction()
	require.NoError(t, err)
	defer tr.Rollback()
	var (
		view *sqldb.TxView
		dtx  = tr
	)
	switch mode {
	case "parallel", "ordered", "fallback":
		if mode == "fallback" {
			// a statement PostgreSQL runs that may write any table puts all the tables of the
			// state of the block in PostgreSQL
			require.NoError(t, tr.ExecSql(`SELECT txid_current()`))
		}
		st, err := sqldb.NewBlockState(tr, []string{"@1TokensTransfer"})
		require.NoError(t, err)
		if mode == "parallel" {
			view = st.NewParallelView(0, "@1TokensTransfer", e.conn)
		} else {
			view = st.NewOrderedView(0, "@1TokensTransfer")
		}
		dtx = view.Transaction()
		require.NoError(t, dtx.Savepoint(`"b7h"`))
	}
	trace := &b7Trace{texts: map[string]int{}}
	sqldb.StatementTrace = trace.add
	defer func() { sqldb.StatementTrace = nil }()
	delete(b7OldIndexes, dtx)
	defer delete(b7OldIndexes, dtx)
	c := &b7Call{impl: impl, sc: b7hSC(dtx, 1)}
	c.sc.FullAccess = s.full
	before := dtx.Stats()
	s.run(c)
	sqldb.StatementTrace = nil
	var out strings.Builder
	out.WriteString(c.log.String())
	out.WriteString(trace.b.String())
	for _, r := range c.sc.RollBackTx {
		fmt.Fprintf(&out, "rollback block %d tx %x table %q id %q data %q hash %s\n", r.BlockId, r.TxHash, r.NameTable, r.TableId, r.Data, hex.EncodeToString(r.DataHash))
	}
	for _, f := range c.sc.FeeCredits {
		fmt.Fprintf(&out, "credit %d %d %s\n", f.Ecosystem, f.Recipient, f.Amount)
	}
	stats := dtx.Stats().Since(before)
	// the count of the indexes of a table in pg_class is a statement that reads a row, which the
	// schema keeps the result of
	fmt.Fprintf(&out, "stats statements %d read %d written %d\n", stats.Statements-int64(trace.pgClass), stats.Read-int64(trace.pgClass), stats.Written)
	if view != nil {
		out.WriteString(view.Digest())
	}
	for text, n := range trace.texts {
		e.shapes[mode+" "+text] += n
	}
	return out.String()
}

// b7Diff is the first line two records differ at, with the lines around it
func b7Diff(a, b string) string {
	la, lb := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(la) || i < len(lb); i++ {
		var x, y string
		if i < len(la) {
			x = la[i]
		}
		if i < len(lb) {
			y = lb[i]
		}
		if x != y {
			return fmt.Sprintf("line %d:\n  want %s\n  got  %s", i+1, x, y)
		}
	}
	return ""
}

// compare runs each scenario in each mode with the old functions and twice with those of the
// package, the second time with what the first left in the caches, and requires the same records
func (e *b7Env) compare(t *testing.T, scenarios []b7Scenario, modes []string) {
	for _, s := range scenarios {
		for _, mode := range modes {
			// a first run fills the caches of the node both ways of running share
			e.run(s, mode, b7Old)
			want := e.run(s, mode, b7Old)
			for pass := 1; pass <= 2; pass++ {
				got := e.run(s, mode, b7New)
				if got != want {
					t.Errorf("%s %s pass %d: the records differ at %s", s.name, mode, pass, b7Diff(want, got))
				}
			}
		}
	}
}

// TestB7Harness: the records of a run of the old functions are the same run after run, as are
// those of the functions of the package, and both are the same
func TestB7Harness(t *testing.T) {
	e := b7Open(t)
	all := append(b7Scenarios(), b7TypedScenarios()...)
	for _, s := range all {
		for _, mode := range b7Modes {
			first := e.run(s, mode, b7Old)
			second := e.run(s, mode, b7Old)
			require.Equal(t, first, second, "%s %s: the old functions record the same twice: %s", s.name, mode, b7Diff(first, second))
		}
	}
	e.compare(t, all, b7Modes)
	texts := make([]string, 0, len(e.shapes))
	for text := range e.shapes {
		texts = append(texts, text)
	}
	sort.Strings(texts)
	if testing.Verbose() {
		for _, text := range texts {
			t.Logf("shape %4d %s", e.shapes[text], text)
		}
	}
	t.Logf("%d shapes of statements", len(texts))
}
