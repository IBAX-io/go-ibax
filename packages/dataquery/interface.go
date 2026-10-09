/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package dataquery

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// MaxElements is the most interface elements one read names
const MaxElements = 100

// The interface kinds: the table of each and the columns of an element of it
var interfaceKinds = map[string]struct {
	table   string
	columns []string
}{
	"page":    {"pages", []string{"ecosystem", "id", "name", "value", "conditions", "app_id", "menu", "validate_count", "validate_mode"}},
	"menu":    {"menu", []string{"ecosystem", "id", "name", "value", "conditions", "title"}},
	"snippet": {"snippets", []string{"ecosystem", "id", "name", "value", "conditions", "app_id"}},
}

// IsInterfaceKind tells whether a kind is page, menu or snippet
func IsInterfaceKind(kind string) bool {
	_, ok := interfaceKinds[kind]
	return ok
}

// SourceHash is the hash of an element's source: the hex SHA-256 of its value
func SourceHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// Element reads one interface element: its source as stored, and its hash. The name may be
// prefixed with its ecosystem (@2main); else the element is of the ecosystem given, else of the
// reader's.
func Element(reader Reader, kind, name string, ecosystem int64) (map[string]any, error) {
	if m := prefixedName.FindStringSubmatch(name); m != nil {
		ecosystem, _ = strconv.ParseInt(m[1], 10, 64)
		name = m[2]
	}
	list, err := Elements(reader, kind, []string{name}, ecosystem)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound(kind + " " + name)
	}
	return list[0], nil
}

// Elements reads the interface elements of the names given, leaving out those that do not exist
func Elements(reader Reader, kind string, names []string, ecosystem int64) ([]map[string]any, error) {
	k, ok := interfaceKinds[kind]
	if !ok {
		return nil, ErrTableNotFound(kind)
	}
	if len(names) == 0 {
		return nil, ErrWhere("no names")
	}
	if len(names) > MaxElements {
		return nil, ErrLimit(strconv.Itoa(len(names)) + " names")
	}
	t, err := Open(reader, k.table, ecosystem)
	if err != nil {
		return nil, err
	}
	in := make([]any, len(names))
	for i, name := range names {
		in[i] = name
	}
	limit := MaxElements
	result, err := t.Query(Query{Columns: k.columns, Where: Where{"name": map[string]any{"$in": in}}, Limit: &limit})
	if err != nil {
		return nil, err
	}
	for _, element := range result.List {
		value, _ := element["value"].(string)
		element["hash"] = SourceHash(value)
	}
	return result.List, nil
}
