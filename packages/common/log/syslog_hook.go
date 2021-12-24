//go:build !windows && !nacl && !plan9

/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package log

import (
	"encoding/json"
	"fmt"
	"os"

	b_syslog "github.com/blackjack/syslog"
	"github.com/sirupsen/logrus"
)

var syslogFacilityPriority map[string]b_syslog.Priority

// SyslogHook to send logs via syslog.
type SyslogHook struct {
	Writer        *b_syslog.Writer
	SyslogNetwork string
	SyslogRaddr   string
}

// NewSyslogHook creats SyslogHook
func NewSyslogHook(appName, facility string) (*SyslogHook, error) {
	b_syslog.Openlog(appName, b_syslog.LOG_PID, syslogFacility(facility))
	return &SyslogHook{nil, "", "localhost"}, nil
}

// Fire the log entry
func (hook *SyslogHook) Fire(entry *logrus.Entry) error {
	line, err := entry.String()
	jsonMap := map[string]interface{}{}
	if err := json.Unmarshal([]byte(line), &jsonMap); err == nil {
		delete(jsonMap, "time")
		delete(jsonMap, "level")
		delete(jsonMap, "fields.time")
		if bString, err := json.Marshal(jsonMap); err == nil {
			line = string(bString)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to read entry, %v", err)
		return err
	}

	switch entry.Level {
	case logrus.PanicLevel:
		{
			b_syslog.Crit(line)
			return nil
		}
	case logrus.FatalLevel:
		{
			b_syslog.Crit(line)
			return nil
		}
	case logrus.ErrorLevel:
		{
			b_syslog.Err(line)
			return nil
		}
	case logrus.WarnLevel:
		{
			b_syslog.Warning(line)
			return nil
		}
	case logrus.InfoLevel:
		{
			b_syslog.Info(line)
			return nil
		}
	case logrus.DebugLevel:
		{
			b_syslog.Debug(line)
			return nil
		}
	default:
		return nil
	}
}

// Levels returns list of levels
func (hook *SyslogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func syslogFacility(facility string) b_syslog.Priority {
	return syslogFacilityPriority[facility]
}

func init() {
	syslogFacilityPriority = map[string]b_syslog.Priority{
		"kern":     b_syslog.LOG_KERN,
		"user":     b_syslog.LOG_USER,
		"mail":     b_syslog.LOG_MAIL,
		"daemon":   b_syslog.LOG_DAEMON,
		"auth":     b_syslog.LOG_AUTH,
		"syslog":   b_syslog.LOG_SYSLOG,
		"lpr":      b_syslog.LOG_LPR,
		"news":     b_syslog.LOG_NEWS,
		"uucp":     b_syslog.LOG_UUCP,
		"cron":     b_syslog.LOG_CRON,
		"authpriv": b_syslog.LOG_AUTHPRIV,
		"ftp":      b_syslog.LOG_FTP,
		"local0":   b_syslog.LOG_LOCAL0,
		"local1":   b_syslog.LOG_LOCAL1,
		"local2":   b_syslog.LOG_LOCAL2,
		"local3":   b_syslog.LOG_LOCAL3,
		"local4":   b_syslog.LOG_LOCAL4,
		"local5":   b_syslog.LOG_LOCAL5,
		"local6":   b_syslog.LOG_LOCAL6,
		"local7":   b_syslog.LOG_LOCAL7,
	}
}
