// +build windows

/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daylight

import (
	"fmt"
	"os/exec"
	"regexp"
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)
		return fmt.Errorf("null")
	}
	log.WithFields(log.Fields{"cmd": "tasklist /fi PID eq " + pid}).Debug("command execution result")
	if ok, _ := regexp.MatchString(`(?i)PID`, string(rez)); !ok {
		return fmt.Errorf("null")
	}
	return nil
}
