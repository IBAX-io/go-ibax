/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package types

import (
	"context"

	log "github.com/sirupsen/logrus"
)

// ClientTxPreprocessor procees tx from client
type ClientTxPreprocessor interface {
	//ProcessClientTranstaction([]byte, int64, *log.Entry) (string, error)
	ProcessClientTxBatches([][]byte, int64, *log.Entry) ([]string, error)
}

// SmartContractRunner run serialized contract
type SmartContractRunner interface {
	RunContract(data, hash []byte, keyID, tnow int64, le *log.Entry) error
}

type DaemonFactory interface {
	GetDaemonsList() []string
	Load(context.Context) error
}
