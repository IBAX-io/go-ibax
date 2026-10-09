/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package dataquery

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTypesOfColumns(t *testing.T) {
	for dataType, want := range map[string]Type{
		"bigint": TypeNumber, "integer": TypeNumber, "numeric": TypeMoney, "double precision": TypeDouble,
		"jsonb": TypeJSON, "boolean": TypeJSON, "timestamp without time zone": TypeTimestamp,
		"timestamp with time zone": TypeTimestamp, "bytea": TypeBytes, "character varying": TypeText,
		"text": TypeText, "uuid": TypeText,
	} {
		assert.Equal(t, want, TypeOf(dataType), dataType)
	}
}

func TestCellsDecodeToTheirJSONValues(t *testing.T) {
	text := func(s string) *string { return &s }
	cells := map[string]any{
		"number":  Decode(text("9007199254740991"), TypeNumber),
		"big":     Decode(text("9007199254740992"), TypeNumber),
		"money":   Decode(text("1000000000000000000000"), TypeMoney),
		"double":  Decode(text("1.5"), TypeDouble),
		"nan":     Decode(text("NaN"), TypeDouble),
		"json":    Decode(text(`{"a":[1,"b"]}`), TypeJSON),
		"bool":    Decode(text("true"), TypeJSON),
		"when":    Decode(text("2026-10-09T08:00:00.000000Z"), TypeTimestamp),
		"bytes":   Decode(text("00ff"), TypeBytes),
		"text":    Decode(text("Ann"), TypeText),
		"null":    Decode(nil, TypeText),
		"invalid": Decode(text("{"), TypeJSON),
	}
	data, err := json.Marshal(cells)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"number": 9007199254740991, "big": "9007199254740992", "money": "1000000000000000000000",
		"double": 1.5, "nan": null, "json": {"a": [1, "b"]}, "bool": true, "when": "2026-10-09T08:00:00.000000Z",
		"bytes": "00ff", "text": "Ann", "null": null, "invalid": null}`, string(data))
}
