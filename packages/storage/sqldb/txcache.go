/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"context"
	"regexp"
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
	if len(c.marks) > 0 {
		c.written = append(c.written, t)
	}
}

func (c *txCache) forgetAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.byTable)
	if len(c.marks) > 0 {
		c.written = append(c.written, allTables)
	}
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
		c.written, c.marks = c.written[:0], c.marks[:0]
		return
	}
	at := c.marks[i].at
	for _, t := range c.written[at:] {
		if t == allTables {
			clear(c.byTable)
			break
		}
		delete(c.byTable, t)
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

var leadingWord = regexp.MustCompile(`^\w+`)

var writeStatement = regexp.MustCompile(`(?is)^\s*(?:insert\s+into|update|delete\s+from)\s+(?:only\s+)?(?:(?:"[^"]+"|\w+)\s*\.\s*)*("[^"]+"|\w+)`)

// written tells the table a statement writes: none for a read, all for one the cache cannot tell
func written(sql string) (table string, none, all bool) {
	s := strings.TrimSpace(sql)
	if strings.Contains(strings.TrimRight(s, "; \t\n"), ";") {
		return "", false, true
	}
	switch strings.ToLower(leadingWord.FindString(s)) {
	case "select", "savepoint", "release", "set", "show":
		return "", true, false
	case "insert", "update", "delete":
		if m := writeStatement.FindStringSubmatch(s); m != nil {
			return m[1], false, false
		}
	}
	return "", false, true
}

// savepointStatement makes the cache follow a statement on savepoints, and tells it is one
func savepointStatement(c *txCache, sql string) bool {
	word := strings.ToLower(leadingWord.FindString(strings.TrimSpace(sql)))
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
