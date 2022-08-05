/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/service/node"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/statsd"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

var (
	// MonitorDaemonCh is monitor daemon channel
	MonitorDaemonCh = make(chan []string, 100)
	NtpDriftFlag    = false
)

type daemon struct {
	goRoutineName string
	sleepTime     time.Duration
	logger        *log.Entry
	atomic        uint32
}

var daemonsList = map[string]func(context.Context, *daemon) error{
	"BlocksCollection":    BlocksCollection,
	"BlockGenerator":      BlockGenerator,
	"Disseminator":        Disseminator,
	"QueueParserTx":       QueueParserTx,
	"QueueParserBlocks":   QueueParserBlocks,
	"Confirmations":       Confirmations,
	"Scheduler":           Scheduler,
	"CandidateNodeVoting": CandidateNodeVoting,
	//"ExternalNetwork":   ExternalNetwork,
}

var rollbackList = []string{
	"BlocksCollection",
	"Confirmations",
}

func daemonLoop(ctx context.Context, goRoutineName string, handler func(context.Context, *daemon) error, retCh chan string) {
	logger := log.WithFields(log.Fields{"daemon_name": goRoutineName})
	defer func() {
		if r := recover(); r != nil {
			logger.WithFields(log.Fields{"type": consts.PanicRecoveredError, "error": r}).Error("panic in daemon")
			panic(r)
		}
	}()

	err := WaitDB(ctx)
	if err != nil {
		return
	}

	d := &daemon{
		goRoutineName: goRoutineName,
		sleepTime:     100 * time.Millisecond,
		logger:        logger,
	}
	idleDelay := time.NewTimer(d.sleepTime)
	//defer idleDelay.Stop()
	for {
		idleDelay.Reset(d.sleepTime)
		select {
		case <-ctx.Done():
			logger.Info("daemon done his work")
			retCh <- goRoutineName
			return
		case <-idleDelay.C:
			MonitorDaemonCh <- []string{d.goRoutineName, converter.Int64ToStr(time.Now().Unix())}
			startTime := time.Now()
			counterName := statsd.DaemonCounterName(goRoutineName)
			handler(ctx, d)
			statsd.Client.TimingDuration(counterName+statsd.Time, time.Now().Sub(startTime), 1.0)
		}
	}
}

// StartDaemons starts daemons
func StartDaemons(ctx context.Context, daemonsToStart []string) {
	go WaitStopTime()

	daemonsTable := make(map[string]string)
	go func() {
		for {
			daemonNameAndTime := <-MonitorDaemonCh
			daemonsTable[daemonNameAndTime[0]] = daemonNameAndTime[1]
			if time.Now().Unix()%10 == 0 {
				log.Debugf("daemonsTable: %v\n", daemonsTable)
			}
		}
	}()

	//go Ntp_Work(ctx)
	// ctx, cancel := context.WithCancel(context.Background())
	// utils.CancelFunc = cancel
	// utils.ReturnCh = make(chan string)

	if conf.Config.TestRollBack {
		daemonsToStart = rollbackList
	}

	log.WithFields(log.Fields{"daemons_to_start": daemonsToStart}).Info("starting daemons")

	for _, name := range daemonsToStart {
		handler, ok := daemonsList[name]
		if ok {
			go daemonLoop(ctx, name, handler, utils.ReturnCh)
			log.WithFields(log.Fields{"daemon_name": name}).Info("started")
			utils.DaemonsCount++
			continue
		}

		log.WithFields(log.Fields{"daemon_name": name}).Warning("unknown daemon name")
	}
}

func getHostPort(h string) string {
	if strings.Contains(h, ":") {
		return h
	}
	return fmt.Sprintf("%s:%d", h, consts.DefaultTcpPort)
}

//ntp
func Ntp_Work(ctx context.Context) {
	var count = 0
	for {
		select {
		case <-ctx.Done():
			log.Error("Ntp_Work done his work")
			return
		case <-time.After(time.Second * 4):
			b, err := utils.CheckClockDrift()
			if err != nil {
				log.WithFields(log.Fields{"daemon_name Ntp_Work err": err.Error()}).Error("Ntp_Work")
			} else {
				if b {
					NtpDriftFlag = true
					count = 0
				} else {
					count++
				}
				if count > 10 {
					var sp sqldb.PlatformParameter
					count, err := sp.GetNumberOfHonorNodes()
					if err != nil {
						log.WithFields(log.Fields{"Ntp_Work GetNumberOfHonorNodes  err": err.Error()}).Error("GetNumberOfHonorNodes")
					} else {
						if NtpDriftFlag && count > 1 {
							NtpDriftFlag = false
						}
					}

				}
			}

		}
	}
}

func generateProcessBlockNew(blockHeader, prevBlock *types.BlockHeader, trs [][]byte, classifyTxsMap map[int][]*transaction.Transaction) error {
	blockBin, err := generateNextBlock(blockHeader, prevBlock, trs)
	if err != nil {
		return err
	}
	//err = block.InsertBlockWOForks(blockBin, true, false)
	err = block.InsertBlockWOForksNew(blockBin, classifyTxsMap, true, false)
	if err != nil {
		log.WithError(err).Error("on inserting new block")
		return err
	}
	log.WithFields(log.Fields{"block": blockHeader.String(), "type": consts.SyncProcess}).Debug("Generated block ID")
	return nil
}

func GetRemoteGoodHosts() ([]string, error) {
	if syspar.IsHonorNodeMode() {
		return node.GetNodesBanService().FilterBannedHosts(syspar.GetRemoteHosts())
	}
	hosts := make([]string, 0)
	candidateNodes, err := sqldb.GetCandidateNode(syspar.SysInt(syspar.NumberNodes))
	if err != nil {
		log.WithError(err).Error("getting candidate node list")
		return nil, err
	}
	for _, node := range candidateNodes {
		hosts = append(hosts, node.TcpAddress)
	}
	return hosts, nil
}
