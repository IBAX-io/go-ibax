// +build windows

/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package log

func (hook *SyslogHook) Fire(entry *logrus.Entry) error {
	return nil
}

func (hook *SyslogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}
