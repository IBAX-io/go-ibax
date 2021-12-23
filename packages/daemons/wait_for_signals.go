/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"github.com/IBAX-io/go-ibax/packages/chain/system"
	"github.com/IBAX-io/go-ibax/packages/statsd"
	"os"
	"os/signal"
	"syscall"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

/*
#include <stdio.h>
#include <signal.h>

extern void go_callback_int();
static inline void SigBreak_Handler(int n_signal){
    printf("closed\n");
	go_callback_int();
}
static inline void waitSig() {
    #if (WIN32 || WIN64)
    signal(SIGBREAK, &SigBreak_Handler);
    signal(SIGINT, &SigBreak_Handler);
    #endif
}
*/
import (
	"C"
)

//export go_callback_int
func go_callback_int() {
	SigChan <- syscall.Signal(1)
}

// SigChan is a channel
var SigChan chan os.Signal

func waitSig() {
	C.waitSig()
}

// WaitForSignals waits for Interrupt os.Kill signals
func WaitForSignals() {
	SigChan = make(chan os.Signal, 1)
	waitSig()
	go func() {
		signal.Notify(SigChan, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM, syscall.SIGQUIT)
		for {
			select {
			case <-SigChan:
				if utils.CancelFunc != nil {
					utils.CancelFunc()
					for i := 0; i < utils.DaemonsCount; i++ {
						name := <-utils.ReturnCh
						log.WithFields(log.Fields{"daemon_name": name}).Debug("daemon stopped")
					}

					log.Debug("Daemons killed")
				}

				if sqldb.DBConn != nil {
					err := sqldb.GormClose()
					if err != nil {
						log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("closing gorm")
					}
				}

				err := system.RemovePidFile()
				if err != nil {
					log.WithFields(log.Fields{
						"type": consts.IOError, "error": err, "path": conf.Config.GetPidPath(),
					}).Error("removing file")
				}
				statsd.Close()
				os.Exit(1)
			}
		}
	}()
}
