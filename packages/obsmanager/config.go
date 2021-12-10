/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package obsmanager

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

const (
	inidDBCommand  = "initDatabase"
	genKeysCommand = "generateKeys"
	startCommand   = "start"
)

// ChildOBSConfig struct to manage child entry
type ChildOBSConfig struct {
	Executable     string
	Name           string
	Directory      string
	DBUser         string
	DBPassword     string
	ConfigFileName string
	LogTo          string
	LogLevel       string
	HTTPPort       int
}

func (c ChildOBSConfig) configCommand() *exec.Cmd {

	args := []string{
		"config",
		fmt.Sprintf("--path=%s", c.configPath()),
		fmt.Sprintf("--dbUser=%s", c.DBUser),
		fmt.Sprintf("--dbPassword=%s", c.DBPassword),
		fmt.Sprintf("--dbName=%s", c.Name),
		fmt.Sprintf("--httpPort=%d", c.HTTPPort),
		fmt.Sprintf("--dataDir=%s", c.Directory),
		fmt.Sprintf("--keysDir=%s", c.Directory),
		fmt.Sprintf("--logTo=%s", c.LogTo),
		fmt.Sprintf("--logLevel=%s", c.LogLevel),
		"--obsMode=OBS",
	}

	return exec.Command(c.Executable, args...)
}

func (c ChildOBSConfig) initDBCommand() *exec.Cmd {
	return c.getCommand(inidDBCommand)
}

func (c ChildOBSConfig) generateKeysCommand() *exec.Cmd {
	return c.getCommand(genKeysCommand)
}

func (c ChildOBSConfig) startCommand() *exec.Cmd {
	return c.getCommand(startCommand)
}

func (c ChildOBSConfig) configPath() string {
	return filepath.Join(c.Directory, c.ConfigFileName)
}

func (c ChildOBSConfig) getCommand(commandName string) *exec.Cmd {
	args := []string{
		commandName,
		fmt.Sprintf("--config=%s", c.configPath()),
	}

	return exec.Command(c.Executable, args...)
}
