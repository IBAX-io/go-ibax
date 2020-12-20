/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpclient

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"

	log "github.com/sirupsen/logrus"
)

var wrongAddressError = errors.New("Wrong address")
func newConnection(addr string) (net.Conn, error) {
	if len(addr) == 0 {
		return nil, wrongAddressError
	}

	host, err := NormalizeHostAddress(addr, consts.DEFAULT_TCP_PORT)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.NetworkError, "host": addr, "error": err}).Error("on normalize host address")
		return nil, err
	}

	conn, err := net.DialTimeout("tcp", host, consts.TCPConnTimeout)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ConnectionError, "error": err, "address": host}).Debug("dialing tcp")
		return nil, err
	}

	conn.SetReadDeadline(time.Now().Add(consts.READ_TIMEOUT * time.Second))
	conn.SetWriteDeadline(time.Now().Add(consts.WRITE_TIMEOUT * time.Second))
	return conn, nil
}
