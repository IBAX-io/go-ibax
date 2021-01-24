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
	return os.Remove(conf.Config.GetPidPath())
}

// ReadPidFile reads pid file
func ReadPidFile() (int, error) {
	pidPath := conf.Config.GetPidPath()
	if _, err := os.Stat(pidPath); err != nil {
}
