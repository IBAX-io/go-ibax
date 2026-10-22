/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"github.com/jackc/pgx/v5/pgtype"
)

// The rows a block writes to a table go in by a single statement of unnest: one parameter per
// column, each an array of one value per row, so that the statement is the same text whatever the
// number of rows, pgx plans it once on a connection, and no block reaches the 65535 parameters of a
// statement

// column is the array of the values f gives of the rows, as a parameter of unnest
func column[R, T any](rows []R, f func(R) T) pgtype.Array[T] {
	values := make([]T, len(rows))
	for i, r := range rows {
		values[i] = f(r)
	}
	return pgtype.Array[T]{
		Elements: values,
		Dims:     []pgtype.ArrayDimension{{Length: int32(len(values)), LowerBound: 1}},
		Valid:    true,
	}
}
