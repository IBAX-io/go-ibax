/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package log

import (
	"encoding/hex"

	"github.com/sirupsen/logrus"
)

	for i := range entry.Data {
		if b, ok := entry.Data[i].([]byte); ok {
			entry.Data[i] = hex.EncodeToString(b)
		}
	}
	return nil
}
