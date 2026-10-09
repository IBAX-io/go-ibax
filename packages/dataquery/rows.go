/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package dataquery

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

// The row scope of a table (docs/api/data-query.md): permissions.rows, a filter of the query
// language whose values may be the reader's, each named in place of a whole value
const (
	readerAccount   = "$account"
	readerKeyID     = "$key_id"
	readerEcosystem = "$ecosystem_id"
	// The roles the reader holds in the table's ecosystem, as a list for $in and $nin
	readerRoles = "$roles"
)

// allColumns are the table's columns as its own row scope names them, readable or not
type allColumns struct{ *Table }

func (allColumns) readable(string) error { return nil }

// readRowScope reads the table's row scope for its reader. A scope that cannot be read gives the
// reader no rows: the table is refused
func (t *Table) readRowScope() error {
	text := t.Record.Permissions.Rows
	if text == "" {
		return nil
	}
	var scope Where
	err := DecodeJSON([]byte(text), &scope)
	if err == nil {
		var bound any
		if bound, err = t.bindReader(scope); err == nil {
			t.rows = bound.(Where)
			// Compiled once, so that a scope naming what the table lacks is refused at once
			_, err = t.visible(&statement{schema: t})
		}
	}
	if err != nil {
		log.WithFields(log.Fields{"type": consts.EvalError, "error": err, "table": t.physical, "rows": text}).
			Warning("reading a row scope")
		return ErrAccessDenied(t.Name)
	}
	return nil
}

// bindReader puts the reader's values in place of their names in a row scope
func (t *Table) bindReader(value any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		bound := make(map[string]any, len(v))
		for key, item := range v {
			b, err := t.bindReader(item)
			if err != nil {
				return nil, err
			}
			bound[key] = b
		}
		return bound, nil
	case []any:
		bound := make([]any, len(v))
		for i, item := range v {
			b, err := t.bindReader(item)
			if err != nil {
				return nil, err
			}
			bound[i] = b
		}
		return bound, nil
	case string:
		if !strings.HasPrefix(v, "$") {
			return v, nil
		}
		switch v {
		case readerAccount:
			return t.reader.AccountID, nil
		case readerKeyID:
			return json.Number(strconv.FormatInt(t.reader.KeyID, 10)), nil
		case readerEcosystem:
			return json.Number(strconv.FormatInt(t.reader.Ecosystem, 10)), nil
		case readerRoles:
			roles, err := sqldb.GetMemberRoles(nil, t.Ecosystem, t.reader.AccountID)
			if err != nil {
				return nil, err
			}
			list := make([]any, len(roles))
			for i, role := range roles {
				list[i] = json.Number(strconv.FormatInt(role, 10))
			}
			return list, nil
		}
		return nil, ErrValue(v)
	}
	return value, nil
}

// visible is the SQL of the rows the reader sees, its values bound in the statement: in a shared
// table those of its ecosystem, and of them those its row scope gives the reader
func (t *Table) visible(s *statement) (string, error) {
	if t.rows == nil {
		return t.scope(), nil
	}
	own := s.schema
	s.schema = allColumns{t}
	defer func() { s.schema = own }()
	rows, err := s.where(t.rows, 0)
	if err != nil {
		return "", err
	}
	return t.scope() + " AND " + rows, nil
}

// Sees tells whether the reader sees the row of an id
func (t *Table) Sees(id string) error {
	s := &statement{schema: allColumns{t}}
	visible, err := t.visible(s)
	if err != nil {
		return err
	}
	key, err := s.resolve("id")
	if err != nil {
		return ErrNotFound(t.Name + " " + id)
	}
	cond, err := s.compare(key, "id", "$eq", id)
	if err != nil {
		return err
	}
	rows, err := query(`SELECT 1 FROM `+t.from()+` WHERE `+visible+` AND `+cond+` LIMIT 1`, s.args...)
	if err != nil {
		return queryFailed(err, t.physical)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return queryFailed(err, t.physical)
		}
		return ErrNotFound(t.Name + " " + id)
	}
	return nil
}
