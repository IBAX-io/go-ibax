/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package modes

import (
	"context"

	"github.com/IBAX-io/go-ibax/packages/api"
	"github.com/IBAX-io/go-ibax/packages/block"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/daemons"
	"github.com/IBAX-io/go-ibax/packages/model"
	"github.com/IBAX-io/go-ibax/packages/network/tcpserver"
	"github.com/IBAX-io/go-ibax/packages/service"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

type BCEcosysLookupGetter struct{}

func (g BCEcosysLookupGetter) GetEcosystemLookup() ([]int64, []string, error) {
	return model.GetAllSystemStatesIDs()
}

type OBSEcosystemLookupGetter struct{}

func (g OBSEcosystemLookupGetter) GetEcosystemLookup() ([]int64, []string, error) {
	return []int64{1}, []string{"Platform ecosystem"}, nil
}

func BuildEcosystemLookupGetter() types.EcosystemLookupGetter {
	if conf.Config.IsSupportingOBS() {
		return OBSEcosystemLookupGetter{}
	}

	return BCEcosysLookupGetter{}
}

type BCEcosysIDValidator struct {
	logger *log.Entry
}

func (v BCEcosysIDValidator) Validate(formEcosysID, clientEcosysID int64, le *log.Entry) (int64, error) {
	if formEcosysID <= 0 {
		return clientEcosysID, nil
	}

	count, err := model.GetNextID(nil, "1_ecosystems")
	if err != nil {
		le.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting next id of ecosystems")
		return 0, err
	}

	if formEcosysID >= count {
		le.WithFields(log.Fields{"state_id": formEcosysID, "count": count, "type": consts.ParameterExceeded}).Error("ecosystem is larger then max count")
		return 0, api.ErrEcosystemNotFound
	}

	return formEcosysID, nil
}

type OBSEcosysIDValidator struct{}

func (OBSEcosysIDValidator) Validate(id, clientID int64, le *log.Entry) (int64, error) {
	return consts.DefaultOBS, nil
}

func GetEcosystemIDValidator() types.EcosystemIDValidator {
	if conf.Config.IsSupportingOBS() {
		return OBSEcosysIDValidator{}
	}

	return BCEcosysIDValidator{}
}

type BCEcosystemNameGetter struct{}

func (ng BCEcosystemNameGetter) GetEcosystemName(id int64) (string, error) {
	ecosystem := &model.Ecosystem{}
	found, err := ecosystem.Get(nil, id)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("on getting ecosystem from db")
		return "", err
	}

	if !found {
		log.WithFields(log.Fields{"type": consts.NotFound, "id": id, "error": api.ErrEcosystemNotFound}).Error("ecosystem not found")
		return "", err
	}

	return ecosystem.Name, nil
}

type OBSEcosystemNameGetter struct{}

func (ng OBSEcosystemNameGetter) GetEcosystemName(id int64) (string, error) {
	return "Platform ecosystem", nil
}

func BuildEcosystemNameGetter() types.EcosystemNameGetter {
	if conf.Config.IsSupportingOBS() {
		return OBSEcosystemNameGetter{}
	}

	return BCEcosystemNameGetter{}
}

// BCDaemonLoader allow load blockchain daemons
type BCDaemonLoader struct {
	logger            *log.Entry
	DaemonListFactory types.DaemonListFactory
}

// Load loads blockchain daemons
func (l BCDaemonLoader) Load(ctx context.Context) error {
	if err := daemons.InitialLoad(l.logger); err != nil {
		return err
	}

	if err := syspar.SysUpdate(nil); err != nil {
		log.Errorf("can't read system parameters: %s", utils.ErrInfo(err))
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
	daemons.StartDaemons(ctx, l.DaemonListFactory.GetDaemonsList())

	if err := tcpserver.TcpListener(conf.Config.TCPServer.Str()); err != nil {
		log.Errorf("can't start tcp servers, stop")
		return err
	}

	na := service.NewNodeRelevanceService()
	na.Run(ctx)

	if err := service.InitNodesBanService(); err != nil {
		l.logger.WithError(err).Error("Can't init ban service")
		return err
	}

	return nil
}

// SNDaemonLoader allows load subnode daemons
type SNDaemonLoader struct {
	logger            *log.Entry
	DaemonListFactory types.DaemonListFactory
}

// Load loads subnode daemons
func (l SNDaemonLoader) Load(ctx context.Context) error {
	daemons.InitialLoad(l.logger)

	if err := syspar.SysUpdate(nil); err != nil {
		log.Errorf("can't read system parameters: %s", utils.ErrInfo(err))
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
	daemons.StartDaemons(ctx, l.DaemonListFactory.GetDaemonsList())

	if err := tcpserver.TcpListener(conf.Config.TCPServer.Str()); err != nil {
		log.Errorf("can't start tcp servers, stop")
		return err
	}
	service.NewNodeRelevanceService().Run(ctx)

	if err := service.InitNodesBanService(); err != nil {
		l.logger.WithError(err).Error("Can't init ban service")
		return err
	}

	return nil
}

// OBSDaemonLoader allows load obs daemons
type OBSDaemonLoader struct {
	logger            *log.Entry
	DaemonListFactory types.DaemonListFactory
}

// Load loads obs daemons
func (l OBSDaemonLoader) Load(ctx context.Context) error {

	if err := syspar.SysUpdate(nil); err != nil {
		l.logger.Errorf("can't read system parameters: %s", utils.ErrInfo(err))
		return err
	}
	if err := syspar.SysTableColType(nil); err != nil {
		log.Errorf("can't table col type: %s", utils.ErrInfo(err))
		return err
	}
	logMode(l.logger, conf.Config.OBSMode)
	l.logger.Info("load contracts")
	if err := smart.LoadContracts(); err != nil {
		l.logger.Errorf("Load Contracts error: %s", err)
		return err
	}

	l.logger.Info("start daemons")
	daemons.StartDaemons(ctx, l.DaemonListFactory.GetDaemonsList())

	//
	if err := tcpserver.TcpListener(conf.Config.TCPServer.Str()); err != nil {
		log.Errorf("can't start tcp servers, stop")
		return err
	}

	return nil
}

func GetDaemonLoader() types.DaemonLoader {
	if conf.Config.IsSupportingOBS() {
		return OBSDaemonLoader{
			logger:            log.WithFields(log.Fields{"loader": "obs_daemon_loader"}),
			DaemonListFactory: OBSDaemonsListFactory{},
		}
	}

	logLevel := log.GetLevel()
	log.SetLevel(log.InfoLevel)
	logger.WithFields(log.Fields{"mode": mode}).Info("Node running mode")
	log.SetLevel(logLevel)
}
