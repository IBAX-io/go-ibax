// +build windows

/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package log

func (hook *SyslogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}
