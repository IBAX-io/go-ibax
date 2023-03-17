/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

const rollbackHistoryLimit = 100

type HistoryResult struct {
	List []map[string]string `json:"list"`
}
