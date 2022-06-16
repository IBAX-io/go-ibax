/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package modes

import (
	"context"

	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/clbmanager"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/daemons"
	"github.com/IBAX-io/go-ibax/packages/network/tcpserver"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

func GetDaemonLoader() types.DaemonFactory {
	if conf.Config.IsSupportingCLB() {
		return CLBDaemonFactory{
			logger: log.WithFields(log.Fields{"loader": "clb_daemon_loader"}),
		}
	}

	if conf.Config.IsSubNode() {
		return SNDaemonFactory{
			logger: log.WithFields(log.Fields{"loader": "subnode_daemon_loader"}),
		}
	}

	return BCDaemonFactory{
		logger: log.WithFields(log.Fields{"loader": "blockchain_daemon_loader"}),
	}
}

// BCDaemonFactory allow load blockchain daemons
type BCDaemonFactory struct {
	logger *log.Entry
}

// Load loads blockchain daemons
func (l BCDaemonFactory) Load(ctx context.Context) error {
	if err := daemons.InitialLoad(l.logger); err != nil {
		return err
	}

	if err := syspar.SysUpdate(nil); err != nil {
		log.Errorf("can't read platform parameters: %s", utils.ErrInfo(err))
		return err
	}
	if err := syspar.SysTableColType(nil); err != nil {
		log.Errorf("can't table col type: %s", utils.ErrInfo(err))
		return err
	}

	if data, ok := block.GetDataFromFirstBlock(); ok {
		syspar.SetFirstBlockData(data)
	}

	mode := "Public blockchain"
	if syspar.IsPrivateBlockchain() {
		mode = "Private Blockchain"
	}

	logMode(l.logger, mode)

	l.logger.Info("load contracts")
	if err := smart.LoadContracts(); err != nil {
		log.Errorf("Load Contracts error: %s", err)
		return err
	}

	l.logger.Info("start daemons")
	daemons.StartDaemons(ctx, l.GetDaemonsList())

	if err := tcpserver.TcpListener(conf.Config.TCPServer.Str()); err != nil {
		log.Errorf("can't start tcp servers, stop")
		return err
	}

	na := node.NewNodeRelevanceService()
	na.Run(ctx)

	if err := node.InitNodesBanService(); err != nil {
		l.logger.WithError(err).Error("Can't init ban service")
		return err
	}

	return nil
}

func (BCDaemonFactory) GetDaemonsList() []string {
	return []string{
		"BlocksCollection",
		"BlockGenerator",
		"QueueParserTx",
		"QueueParserBlocks",
		"Disseminator",
		"Confirmations",
		"Scheduler",
		"CandidateNodeVoting",
		//"ExternalNetwork",
	}
}

// SNDaemonFactory allows load subnode daemons
type SNDaemonFactory struct {
	logger *log.Entry
}

// Load loads subnode daemons
func (l SNDaemonFactory) Load(ctx context.Context) error {
	daemons.InitialLoad(l.logger)

	if err := syspar.SysUpdate(nil); err != nil {
		log.Errorf("can't read platform parameters: %s", utils.ErrInfo(err))
		return err
	}
	if err := syspar.SysTableColType(nil); err != nil {
		log.Errorf("can't table col type: %s", utils.ErrInfo(err))
		return err
	}
	if data, ok := block.GetDataFromFirstBlock(); ok {
		syspar.SetFirstBlockData(data)
	}

	mode := "Public blockchain"
	if syspar.IsPrivateBlockchain() {
		mode = "Private Blockchain"
	}

	logMode(l.logger, mode)

	l.logger.Info("load contracts")
	if err := smart.LoadContracts(); err != nil {
		log.Errorf("Load Contracts error: %s", err)
		return err
	}

	l.logger.Info("start subnode daemons")
	daemons.StartDaemons(ctx, l.GetDaemonsList())

	if err := tcpserver.TcpListener(conf.Config.TCPServer.Str()); err != nil {
		log.Errorf("can't start tcp servers, stop")
		return err
	}
	node.NewNodeRelevanceService().Run(ctx)

	if err := node.InitNodesBanService(); err != nil {
		l.logger.WithError(err).Error("Can't init ban service")
		return err
	}

	return nil
}

func (SNDaemonFactory) GetDaemonsList() []string {
	return []string{
		"Scheduler",
	}
}

// CLBDaemonFactory allows load clb daemons
type CLBDaemonFactory struct {
	logger *log.Entry
}

// Load loads clb daemons
func (l CLBDaemonFactory) Load(ctx context.Context) error {
	if err := syspar.SysUpdate(nil); err != nil {
		l.logger.Errorf("can't read platform parameters: %s", utils.ErrInfo(err))
		return err
	}
	if err := syspar.SysTableColType(nil); err != nil {
		log.Errorf("can't table col type: %s", utils.ErrInfo(err))
		return err
	}
	logMode(l.logger, conf.Config.LocalConf.RunNodeMode)
	l.logger.Info("load contracts")
	if err := smart.LoadContracts(); err != nil {
		l.logger.Errorf("Load Contracts error: %s", err)
		return err
	}

	l.logger.Info("start daemons")
	daemons.StartDaemons(ctx, l.GetDaemonsList())

	//
	if err := tcpserver.TcpListener(conf.Config.TCPServer.Str()); err != nil {
		log.Errorf("can't start tcp servers, stop")
		return err
	}
	clbmanager.InitCLBManager()
	return nil
}

func (CLBDaemonFactory) GetDaemonsList() []string {
	return []string{
		"Scheduler",
	}
}

func logMode(logger *log.Entry, mode string) {
	logLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel)
	logger.WithFields(log.Fields{"mode": mode}).Info("Node running mode")
	log.SetLevel(logLevel)
}
