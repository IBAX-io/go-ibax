/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package log

import (
	"encoding/hex"

	"github.com/sirupsen/logrus"
)

type HexHook struct{}

// Levels returns all log levels
func (hook HexHook) Levels() []logrus.Level {
	return logrus.AllLevels
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
