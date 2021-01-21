/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package log

import (
}

// Fire the log entry
func (hook HexHook) Fire(entry *logrus.Entry) error {
	for i := range entry.Data {
		if b, ok := entry.Data[i].([]byte); ok {
			entry.Data[i] = hex.EncodeToString(b)
		}
	}
	return nil
}
