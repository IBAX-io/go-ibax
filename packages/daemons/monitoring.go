/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

// Monitoring starts monitoring
func Monitoring(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer

	infoBlock := &sqldb.InfoBlock{}
	_, err := infoBlock.Get()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting info block")
		logError(w, fmt.Errorf("can't get info block: %s", err))
		return
	}
	addKey(&buf, "info_block_id", infoBlock.BlockID)
	addKey(&buf, "info_block_hash", converter.BinToHex(infoBlock.Hash))
	addKey(&buf, "info_block_time", infoBlock.Time)
	addKey(&buf, "info_block_key_id", infoBlock.KeyID)
	addKey(&buf, "info_block_ecosystem_id", infoBlock.EcosystemID)
	addKey(&buf, "info_block_node_position", infoBlock.NodePosition)
	addKey(&buf, "honor_nodes_count", syspar.GetNumberOfNodes())

	block := &sqldb.BlockChain{}
	_, err = block.GetMaxBlock()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting max block")
		logError(w, fmt.Errorf("can't get max block: %s", err))
		return
	}
	addKey(&buf, "last_block_id", block.ID)
	addKey(&buf, "last_block_hash", converter.BinToHex(block.Hash))
	addKey(&buf, "last_block_time", block.Time)
	addKey(&buf, "last_block_wallet", block.KeyID)
	addKey(&buf, "last_block_state", block)
	addKey(&buf, "last_block_transactions", block.Tx)

	trCount, err := sqldb.GetTransactionCountAll()
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting transaction count all")
		logError(w, fmt.Errorf("can't get transactions count: %s", err))
		return
	}
	addKey(&buf, "transactions_count", trCount)

	w.Write(buf.Bytes())
}

func addKey(buf *bytes.Buffer, key string, value any) error {
	val, err := converter.InterfaceToStr(value)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("%s\t%s\n", key, val)
	buf.Write([]byte(line))
	return nil
}

func logError(w http.ResponseWriter, err error) {
	w.Write([]byte(err.Error()))
	return
}
