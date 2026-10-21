/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/service/bftmon"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

// bftMetric is the monitoring of the consensus of a chain of the BFT consensus (F18 design 4.9):
// the state of the driver, the commits of the last bftmon.EscalationHeights heights by round, and
// the escalation rate, the share of them committed after round 0
type bftMetric struct {
	bftmon.State
	Heights        int64           `json:"heights"`
	Rounds         map[int64]int64 `json:"rounds"`
	EscalationRate float64         `json:"escalation_rate"`
}

func bftMetricsHandler(w http.ResponseWriter, r *http.Request) {
	if !syspar.IsBftConsensus() {
		errorResponse(w, errNotFound)
		return
	}
	info := &sqldb.InfoBlock{}
	if _, err := info.Get(); err != nil {
		getLogger(r).WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting the info block")
		errorResponse(w, err, http.StatusInternalServerError)
		return
	}
	rounds, err := sqldb.BftRounds(info.BlockID - bftmon.EscalationHeights)
	if err != nil {
		getLogger(r).WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("counting the commits by round")
		errorResponse(w, err, http.StatusInternalServerError)
		return
	}
	m := bftMetric{State: bftmon.Get(), Rounds: rounds}
	for _, n := range rounds {
		m.Heights += n
	}
	if m.Heights > 0 {
		m.EscalationRate = float64(m.Heights-rounds[0]) / float64(m.Heights)
	}
	jsonResponse(w, m)
}
