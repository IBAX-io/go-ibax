/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"strings"
	"sync"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// txCache keeps, for one database transaction (the transaction of a block), the results of the
// reads that the contracts repeat: the rows of 1_tables, the parameters, the control mode of an
// ecosystem, the highest id of a table, the number of indexes of a table. A result depends on one
// table, and goes when a statement writes that table; a statement the cache cannot tell the table
// of and a change of the schema empty it. A rollback to a savepoint takes out the tables written
// since the savepoint. A result is thus always what the database would answer, and the execution
// of a block is the same with the cache or without.
type txCache struct {
	mu      sync.Mutex
	byTable map[string]map[string]any
	// written journals, while savepoints are open, the tables the statements wrote: allTables for
	// a statement that may have written any
	written []string
	marks   []cacheMark
	// gens count, by table, the times the cache forgot the table, and allGen the times it forgot
	// all: a table whose generation has not moved is as it was (generation)
	gens   map[string]uint64
	allGen uint64
}

type cacheMark struct {
	name string
	at   int
}

const allTables = "*"

// schemaTable is the dependency of the results that only a change of the schema changes
const schemaTable = ""

// txCacheKey keys the cache in the context of a database transaction
type txCacheKey struct{}

func withTxCache(ctx context.Context) (context.Context, *txCache) {
	c := &txCache{byTable: make(map[string]map[string]any)}
	return context.WithValue(ctx, txCacheKey{}, c), c
}

func cacheTable(table string) string {
	return strings.ToLower(strings.Trim(table, `"`))
}

func (c *txCache) get(table, key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.byTable[cacheTable(table)][key]
	return v, ok
}

func (c *txCache) put(table, key string, v any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := cacheTable(table)
	if c.byTable[t] == nil {
		c.byTable[t] = make(map[string]any)
	}
	c.byTable[t][key] = v
}

func (c *txCache) forget(table string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := cacheTable(table)
	delete(c.byTable, t)
	c.bump(t)
	if len(c.marks) > 0 {
		c.written = append(c.written, t)
	}
}

func (c *txCache) forgetAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.byTable)
	c.allGen++
	if len(c.marks) > 0 {
		c.written = append(c.written, allTables)
	}
}

func (c *txCache) bump(t string) {
	if c.gens == nil {
		c.gens = make(map[string]uint64)
	}
	c.gens[t]++
}

// generation is the generation of the table: it moves whenever a statement may have written the
// table, or a rollback to a savepoint took back such a statement
func (c *txCache) generation(table string) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.allGen + c.gens[cacheTable(table)]
}

func (c *txCache) savepoint(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.marks = append(c.marks, cacheMark{name: name, at: len(c.written)})
}

func (c *txCache) lastMark(name string) int {
	for i := len(c.marks) - 1; i >= 0; i-- {
		if c.marks[i].name == name {
			return i
		}
	}
	return -1
}

// rollbackTo takes out the tables written since the savepoint; all of them for a savepoint the
// cache does not know
func (c *txCache) rollbackTo(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	i := c.lastMark(name)
	if i < 0 {
		clear(c.byTable)
		c.allGen++
		c.written, c.marks = c.written[:0], c.marks[:0]
		return
	}
	at := c.marks[i].at
	for _, t := range c.written[at:] {
		if t == allTables {
			clear(c.byTable)
			c.allGen++
			break
		}
		delete(c.byTable, t)
		c.bump(t)
	}
	c.written = c.written[:at]
	c.marks = c.marks[:i+1]
}

func (c *txCache) release(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i := c.lastMark(name); i >= 0 {
		c.marks = c.marks[:i]
	}
	if len(c.marks) == 0 {
		c.written = c.written[:0]
	}
}

// cached is the result of load for the key, depending on table, from the cache of the transaction
// if it has it. A transaction StartTransaction did not begin has no cache, and loads each time.
// The result is a read of the table in the record of the statements of the transaction.
func cached[T any](tr *DbTransaction, table, key string, load func() (T, error)) (T, error) {
	if tr == nil || tr.cache == nil {
		return load()
	}
	if v, ok := tr.cache.get(table, key); ok {
		metrics.DBCache.WithLabelValues("hit").Inc()
		return v.(T), nil
	}
	name := cacheTable(table)
	if tr.view != nil {
		if v, ok := tr.view.shared(name, key); ok {
			metrics.DBCache.WithLabelValues("shared").Inc()
			tr.cache.put(table, key, v)
			return v.(T), nil
		}
	}
	metrics.DBCache.WithLabelValues("miss").Inc()
	v, err := load()
	if err == nil {
		tr.cache.put(table, key, v)
		if tr.view != nil {
			tr.view.share(name, key, v)
		}
	}
	return v, err
}

// leadingWord is the match of ^\w+ in s: the ASCII letters, digits and underscores it starts with
func leadingWord(s string) string {
	i := 0
	for i < len(s) && isWordByte(s[i]) {
		i++
	}
	return s[:i]
}

func isWordByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}

// writtenTable is the submatch of the regular expression
// (?is)^\s*(?:insert\s+into|update|delete\s+from)\s+(?:only\s+)?(?:(?:"[^"]+"|\w+)\s*\.\s*)*("[^"]+"|\w+)
// in s: the last name of the table an insert, update or delete writes. Under (?i) the letter s also
// matches the long s (U+017F), and \w the long s and the Kelvin sign (U+212A); \s is the ASCII
// space, tab, newline, form feed and carriage return
func writtenTable(s string) (string, bool) {
	start := skipSpaces(s, 0)
	var p int
	if e := foldedKeyword(s, start, "update"); e >= 0 {
		p = e
	} else if e := foldedKeyword(s, start, "insert"); e >= 0 {
		p = foldedKeyword(s, spaced(s, e), "into")
	} else {
		p = foldedKeyword(s, spaced(s, foldedKeyword(s, start, "delete")), "from")
	}
	if p = spaced(s, p); p < 0 {
		return "", false
	}
	// only is optional and greedy: without it, it can be the name
	if o := foldedKeyword(s, p, "only"); o >= 0 {
		if q := spaced(s, o); q >= 0 {
			if table, ok := qualifiedLast(s, q); ok {
				return table, true
			}
		}
	}
	return qualifiedLast(s, p)
}

// qualifiedLast is the last name of the names separated with dots at p of s, the name before the
// last dot when no name follows it
func qualifiedLast(s string, p int) (string, bool) {
	last, lastEnd := -1, -1
	for {
		e := sqlName(s, p)
		if e < 0 {
			break
		}
		q := skipSpaces(s, e)
		if q == len(s) || s[q] != '.' {
			return s[p:e], true
		}
		last, lastEnd = p, e
		p = skipSpaces(s, q+1)
	}
	if last < 0 {
		return "", false
	}
	return s[last:lastEnd], true
}

// sqlName is the end of the name "[^"]+" or \w+ (under (?i)) at p of s, -1 when there is none
func sqlName(s string, p int) int {
	if p >= len(s) {
		return -1
	}
	if s[p] == '"' {
		q := strings.IndexByte(s[p+1:], '"')
		if q <= 0 {
			return -1
		}
		return p + 1 + q + 1
	}
	e := p
	for e < len(s) {
		n := foldedWordLen(s, e)
		if n == 0 {
			break
		}
		e += n
	}
	if e == p {
		return -1
	}
	return e
}

// foldedWordLen is the length of the character of \w under (?i) at i of s, 0 for another one
func foldedWordLen(s string, i int) int {
	switch {
	case isWordByte(s[i]):
		return 1
	case strings.HasPrefix(s[i:], "\u017f"):
		return 2
	case strings.HasPrefix(s[i:], "\u212a"):
		return 3
	}
	return 0
}

// foldedKeyword is the end of the lower case keyword at p of s matched under (?i), -1 when it is not
// there
func foldedKeyword(s string, p int, keyword string) int {
	if p < 0 {
		return -1
	}
	for i := 0; i < len(keyword); i++ {
		c := keyword[i]
		switch {
		case p < len(s) && s[p]|0x20 == c:
			p++
		case c == 's' && strings.HasPrefix(s[p:], "\u017f"):
			p += 2
		case c == 'k' && strings.HasPrefix(s[p:], "\u212a"):
			p += 3
		default:
			return -1
		}
	}
	return p
}

// spaced is the index after the characters matched by \s+ at p of s, -1 when there is none or p is
// -1
func spaced(s string, p int) int {
	if p < 0 {
		return -1
	}
	if q := skipSpaces(s, p); q > p {
		return q
	}
	return -1
}

// skipSpaces is the index of the first character from p of s that is not matched by \s
func skipSpaces(s string, p int) int {
	for p < len(s) {
		switch s[p] {
		case ' ', '\t', '\n', '\f', '\r':
			p++
		default:
			return p
		}
	}
	return p
}

// written tells the table a statement writes: none for a read, all for one the cache cannot tell
func written(sql string) (table string, none, all bool) {
	s := strings.TrimSpace(sql)
	if strings.Contains(strings.TrimRight(s, "; \t\n"), ";") {
		return "", false, true
	}
	switch strings.ToLower(leadingWord(s)) {
	case "select", "savepoint", "release", "set", "show":
		return "", true, false
	case "insert", "update", "delete":
		if table, ok := writtenTable(s); ok {
			return table, false, false
		}
	}
	return "", false, true
}

// savepointStatement makes the cache follow a statement on savepoints, and tells it is one
func savepointStatement(c *txCache, sql string) bool {
	word := strings.ToLower(leadingWord(strings.TrimSpace(sql)))
	if word != "savepoint" && word != "release" && word != "rollback" {
		return false
	}
	toks := tokenize(sql)
	switch word {
	case "savepoint":
		c.savepoint(savepointName(toks[1:]))
	case "release":
		c.release(savepointName(toks[1:]))
	default:
		if len(toks) < 2 || toks[1].keyword() != "to" {
			return false
		}
		c.rollbackTo(savepointName(toks[2:]))
	}
	return true
}

// registerTxCache makes each statement of a transaction empty, in its cache, what it may change
func registerTxCache(db *gorm.DB) error {
	cacheOf := func(db *gorm.DB) *txCache {
		if db.Statement.Context == nil {
			return nil
		}
		c, _ := db.Statement.Context.Value(txCacheKey{}).(*txCache)
		return c
	}
	byModel := func(db *gorm.DB) {
		if c := cacheOf(db); c != nil {
			if db.Statement.Table == "" {
				c.forgetAll()
				return
			}
			c.forget(db.Statement.Table)
		}
	}
	bySQL := func(db *gorm.DB) {
		c := cacheOf(db)
		if c == nil {
			return
		}
		sql := db.Statement.SQL.String()
		if savepointStatement(c, sql) {
			return
		}
		table, none, all := written(sql)
		switch {
		case none:
		case all:
			c.forgetAll()
		default:
			c.forget(table)
		}
	}
	cb := db.Callback()
	for _, err := range []error{
		cb.Create().After("gorm:create").Register("ibax:cache_create", byModel),
		cb.Update().After("gorm:update").Register("ibax:cache_update", byModel),
		cb.Delete().After("gorm:delete").Register("ibax:cache_delete", byModel),
		cb.Query().After("gorm:query").Register("ibax:cache_query", bySQL),
		cb.Row().After("gorm:row").Register("ibax:cache_row", bySQL),
		cb.Raw().After("gorm:raw").Register("ibax:cache_raw", bySQL),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

type firstRow[T any] struct {
	found bool
	row   T
}

// cachedFirst is find, which reads into dest the row of table the key names, from the cache of the
// transaction if it has it. A dest with an id is read each time, as the id is then a condition of
// the read.
func cachedFirst[T any](tr *DbTransaction, table, key string, dest *T, hasID bool, find func() (bool, error)) (bool, error) {
	if hasID {
		return find()
	}
	r, err := cached(tr, table, key, func() (firstRow[T], error) {
		found, err := find()
		return firstRow[T]{found: found, row: *dest}, err
	})
	if err != nil {
		return false, err
	}
	if r.found {
		*dest = r.row
	}
	return r.found, nil
}

// maxIDKey keys the highest id of a table, 0 for a table without rows
const maxIDKey = "maxid"

// maxID is the highest id of the table
func (tr *DbTransaction) maxID(table string) (int64, error) {
	return tr.MaxID(table)
}

// MaxID is the highest id of the table in the transaction
func (tr *DbTransaction) MaxID(table string) (int64, error) {
	return cached(tr, table, maxIDKey, func() (int64, error) {
		var id int64
		rows, err := tr.queryRaw(`select id from "` + table + `" order by id desc limit 1`)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err, "table": table}).Error("selecting next id from table")
			return 0, err
		}
		rows.Next()
		rows.Scan(&id)
		rows.Close()
		return id, nil
	})
}

// CachedMaxID is the highest id of the table the cache of the transaction has
func (tr *DbTransaction) CachedMaxID(table string) (int64, bool) {
	if tr == nil || tr.cache == nil {
		return 0, false
	}
	v, ok := tr.cache.get(table, maxIDKey)
	if !ok {
		return 0, false
	}
	return v.(int64), true
}

// KeepMaxID puts back in the cache the highest id of the table after a write that the caller
// knows leaves it at id: the write itself made the cache forget the table
func (tr *DbTransaction) KeepMaxID(table string, id int64) {
	if tr == nil || tr.cache == nil {
		return
	}
	tr.cache.put(table, maxIDKey, id)
}
