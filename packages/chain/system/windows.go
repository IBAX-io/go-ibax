//go:build windows

/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package system

import (
	"fmt"
	"os/exec"
	"regexp"
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

// KillPid kills the process with the specified pid
func KillPid(pid string) error {
	if sqldb.DBConn != nil {
		sd := &sqldb.StopDaemon{StopTime: time.Now().Unix()}
		err := sd.Create()
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Error creating StopDaemon")
			return err
		}
	}
	rez, err := exec.Command("tasklist", "/fi", "PID eq "+pid).Output()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.CommandExecutionError, "err": err, "cmd": "tasklist /fi PID eq" + pid}).Error("Error executing command")
		return err
	}
	if string(rez) == "" {
		return fmt.Errorf("null")
	}
	log.WithFields(log.Fields{"cmd": "tasklist /fi PID eq " + pid}).Debug("command execution result")
	if ok, _ := regexp.MatchString(`(?i)PID`, string(rez)); !ok {
		return fmt.Errorf("null")
	}
	return nil
}
