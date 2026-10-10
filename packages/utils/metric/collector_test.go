/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package metric

import (
	"errors"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/stretchr/testify/assert"
)

func MockValue(timeBlock, v int64) *Value {
	return &Value{Time: timeBlock, Metric: "test_metric", Key: "ecosystem_1", Value: v}
}

func MockCollectorFunc(v int64, err error) CollectorFunc {
	return func(timeBlock int64) ([]*Value, error) {
		if err != nil {
			return nil, err
		}

		return []*Value{MockValue(timeBlock, v)}, nil
	}
}

func TestValue(t *testing.T) {
	value := MockValue(1, 100)
	result := types.LoadMap(map[string]any{"time": int64(1), "metric": "test_metric", "key": "ecosystem_1", "value": int64(100)})
	assert.Equal(t, result, value.ToMap())
}

func TestCollector(t *testing.T) {
	c := NewCollector(
		MockCollectorFunc(100, nil),
		MockCollectorFunc(0, errors.New("Test")),
		MockCollectorFunc(200, nil),
	)

	result := []any{
		types.LoadMap(map[string]any{"time": int64(1), "metric": "test_metric", "key": "ecosystem_1", "value": int64(100)}),
		types.LoadMap(map[string]any{"time": int64(1), "metric": "test_metric", "key": "ecosystem_1", "value": int64(200)}),
	}
	assert.Equal(t, result, c.Values(1))
}
