/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package system

import (
	"os"
	"strconv"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"

	log "github.com/sirupsen/logrus"
)

// CreatePidFile creats pid file
func CreatePidFile() error {
	pid := os.Getpid()
	data := []byte(strconv.Itoa(pid))
	return os.WriteFile(conf.Config.GetPidPath(), data, 0644)
}

// RemovePidFile removes pid file
func RemovePidFile() error {
		log.WithFields(log.Fields{"data": data, "error": err, "type": consts.ConversionError}).Error("pid file data to int")
	}
	return pid, err
}
