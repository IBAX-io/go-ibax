/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// contractCall is a contract of ecosystem 1 and the text of its parameters, which {i} in a value
// makes unlike in each transaction
type contractCall struct {
	id     int
	types  map[string]string
	params map[string]string
}

// newContractCall reads the parameters NAME=VALUE of a contract and checks them against its fields
func newContractCall(n *node, name string, args []string) (*contractCall, error) {
	var info struct {
		ID     int `json:"id"`
		Fields []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"fields"`
	}
	if err := n.get("contract/"+url.PathEscape(name), &info); err != nil {
		return nil, err
	}
	call := &contractCall{id: info.ID, types: map[string]string{}, params: map[string]string{}}
	for _, f := range info.Fields {
		call.types[f.Name] = f.Type
	}
	for _, arg := range args {
		k, v, ok := strings.Cut(arg, "=")
		if !ok {
			return nil, fmt.Errorf("%q is not NAME=VALUE", arg)
		}
		if _, ok := call.types[k]; !ok {
			return nil, fmt.Errorf("%s has no parameter %s", name, k)
		}
		call.params[k] = v
	}
	return call, nil
}

// values are the parameters of transaction i, of the types of the fields of the contract
func (c *contractCall) values(i int) (map[string]any, error) {
	out := make(map[string]any, len(c.params))
	for k, text := range c.params {
		text = strings.ReplaceAll(text, "{i}", strconv.Itoa(i))
		v, err := fieldValue(c.types[k], text)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		out[k] = v
	}
	return out, nil
}

// fieldValue is a parameter of a type of the contract language, from its text: an array and a map
// as JSON, bytes as hex
func fieldValue(typ, text string) (any, error) {
	switch typ {
	case "int", "address":
		return strconv.ParseInt(text, 10, 64)
	case "float":
		return strconv.ParseFloat(text, 64)
	case "bool":
		return strconv.ParseBool(text)
	case "string", "money":
		return text, nil
	case "bytes":
		return hex.DecodeString(text)
	case "array":
		var v []any
		err := json.Unmarshal([]byte(text), &v)
		return v, err
	case "map":
		var v map[string]any
		err := json.Unmarshal([]byte(text), &v)
		return v, err
	}
	return nil, fmt.Errorf("parameters of type %s are not supported", typ)
}
