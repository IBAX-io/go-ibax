/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package metric

import (
	"strconv"
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

const (
	metricEcosystemPages   = "ecosystem_pages"
	metricEcosystemMembers = "ecosystem_members"
	metricEcosystemTx      = "ecosystem_tx"
)

// CollectMetricDataForEcosystemTables returns metrics for some tables of ecosystems
func CollectMetricDataForEcosystemTables(timeBlock int64) (metricValues []*Value, err error) {
	stateIDs, _, err := sqldb.GetAllSystemStatesIDs()
	if err != nil {
		log.WithFields(log.Fields{"error": err, "type": consts.DBError}).Error("get all system states ids")
		return nil, err
	}

	now := time.Unix(timeBlock, 0)
	unixDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).Unix()

	for _, stateID := range stateIDs {
		var pagesCount, membersCount int64

		tablePrefix := strconv.FormatInt(stateID, 10)

		p := &sqldb.Page{}
		p.SetTablePrefix(tablePrefix)
		if pagesCount, err = p.Count(); err != nil {
			log.WithFields(log.Fields{"error": err, "type": consts.DBError}).Error("get count of pages")
			return nil, err
		}
		metricValues = append(metricValues, &Value{
			Time:   unixDate,
			Metric: metricEcosystemPages,
			Key:    tablePrefix,
			Value:  pagesCount,
		})

		m := &sqldb.Member{}
		m.SetTablePrefix(tablePrefix)
		if membersCount, err = m.Count(); err != nil {
			log.WithFields(log.Fields{"error": err, "type": consts.DBError}).Error("get count of members")
			return nil, err
		}
		metricValues = append(metricValues, &Value{
			Time:   unixDate,
			Metric: metricEcosystemMembers,
			Key:    tablePrefix,
			Value:  membersCount,
		})
	}

	return metricValues, nil
}

// CollectMetricDataForEcosystemTx returns metrics for transactions of ecosystems
func CollectMetricDataForEcosystemTx(timeBlock int64) (metricValues []*Value, err error) {
	ecosystemTx, err := sqldb.GetEcosystemTxPerDay(timeBlock)
	if err != nil {
		log.WithFields(log.Fields{"error": err, "type": consts.DBError}).Error("get ecosystem transactions by period")
		return nil, err
	}
	for _, item := range ecosystemTx {
		if len(item.Ecosystem) == 0 {
			continue
		}

		metricValues = append(metricValues, &Value{
			Time:   item.UnixTime,
			Metric: metricEcosystemTx,
			Key:    item.Ecosystem,
			Value:  item.Count,
		})
	}

	return metricValues, nil
}
