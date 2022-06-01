package main

import (
	"flag"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/tools/desync_monitor/config"
	"github.com/IBAX-io/go-ibax/tools/desync_monitor/query"

	log "github.com/sirupsen/logrus"
)

const confPathFlagName = "confPath"
const nodesListFlagName = "nodesList"
const daemonModeFlagName = "daemonMode"
const queryingPeriodFlagName = "queryingPeriod"

var configPath = flag.String(confPathFlagName, "config.toml", "path to desync monitor config")
var nodesList = flag.String(nodesListFlagName, "127.0.0.1:7079", "which nodes to query, in format url1,url2,url3")
var daemonMode = flag.Bool(daemonModeFlagName, false, "start as daemon")
var queryingPeriod = flag.Int(queryingPeriodFlagName, 1, "period of querying nodes in seconds, if started as daemon")

func minElement(slice []int64) int64 {
	var min int64 = math.MaxInt64
	for _, blockID := range slice {
		if blockID < min {
			min = blockID
		}
	}
	return min
}

func flagsOverrideConfig(conf *config.Config) {
	flag.Visit(func(flag *flag.Flag) {
		switch flag.Name {
		case nodesListFlagName:
			nodesList := strings.Split(*nodesList, ",")
			conf.NodesList = nodesList
		case daemonModeFlagName:
			conf.Daemon.DaemonMode = *daemonMode
		case queryingPeriodFlagName:
			conf.Daemon.QueryingPeriod = *queryingPeriod
		}
	})
}

func monitor(conf *config.Config) {
	maxBlockIDs, err := query.MaxBlockIDs(conf.NodesList)
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Error("on sending max block request")
		return
	}

	log.Infoln("max blocks ", maxBlockIDs)

	blockInfos, err := query.BlockInfo(conf.NodesList, minElement(maxBlockIDs))
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Error("on sending block info request")
		return
	}

	hash2Node := map[string][]string{}
	for node, blockInfo := range blockInfos {
		rollbacksHash := fmt.Sprintf("%d: %x", blockInfo.BlockID, blockInfo.RollbacksHash)
		if _, ok := hash2Node[rollbacksHash]; !ok {
			hash2Node[rollbacksHash] = []string{}
		}
		hash2Node[rollbacksHash] = append(hash2Node[rollbacksHash], node)
	}

	log.Infof("requested nodes: %v", conf.NodesList)

	if len(hash2Node) <= 1 {
		log.Infoln("nodes synced")
		return
	}

	hash2NodeStrResults := []string{}
	for k, v := range hash2Node {
		hash2NodeStrResults = append(hash2NodeStrResults, fmt.Sprintf("%s: %s", k, v))
	}

	log.Infof("nodes unsynced. Rollback hashes are: %s", strings.Join(hash2NodeStrResults, ", "))
}

func main() {
	flag.Parse()
	conf := &config.Config{}
	if err := conf.Read(*configPath); err != nil {
		log.WithError(err).Fatal("reading config")
	}

	flagsOverrideConfig(conf)

	if conf.Daemon.DaemonMode {
		log.Infoln("MODE: daemon")
		ticker := time.NewTicker(time.Second * time.Duration(conf.Daemon.QueryingPeriod))
		for range ticker.C {
			monitor(conf)
		}
	} else {
		log.Println("MODE: single request")
		monitor(conf)
	}
}
