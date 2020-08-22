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
