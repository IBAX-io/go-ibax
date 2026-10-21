/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

// LargestTxSize returns the byte length of the largest transaction in txs.
func LargestTxSize(txs [][]byte) int {
	largest := 0
	for _, tx := range txs {
		if len(tx) > largest {
			largest = len(tx)
		}
	}
	return largest
}
