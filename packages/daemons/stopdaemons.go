/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"time"

	"github.com/IBAX-io/go-ibax/packages/chain/system"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

// WaitStopTime closes the database and stop daemons
func WaitStopTime() {
	var first bool
	for {
		if model.DBConn == nil {
			time.Sleep(time.Second * 3)
			continue
		}
		if !first {
			err := model.Delete(nil, "stop_daemons", "")
			if err != nil {
				log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("deleting from stop daemons")
			}
			first = true
		}
		dExists, err := model.Single(nil, `SELECT stop_time FROM stop_daemons`).Int64()
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("selecting stop_time from StopDaemons")
		}
		if dExists > 0 {
			utils.CancelFunc()
			for i := 0; i < utils.DaemonsCount; i++ {
				name := <-utils.ReturnCh
				log.WithFields(log.Fields{"daemon_name": name}).Debug("daemon stopped")
			}

			err := model.GormClose()
			if err != nil {
				log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("gorm close")
			}
			err = system.RemovePidFile()
			if err != nil {
				log.WithFields(log.Fields{
					"type": consts.IOError, "error": err,
				}).Error("removing pid file")
				panic(err)
			}
		}
		time.Sleep(time.Second)
	}
}
