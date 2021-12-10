//go:build !android && !ios

/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package chain

import (
	"net"
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"

	log "github.com/sirupsen/logrus"
)

func httpListener(ListenHTTPHost string, route http.Handler) {
	l, err := net.Listen("tcp", ListenHTTPHost)
	log.WithFields(log.Fields{"host": ListenHTTPHost, "type": consts.NetworkError}).Debug("trying to listen at")
	if err == nil {
		log.WithFields(log.Fields{"host": ListenHTTPHost}).Info("listening at")
	} else {
		log.WithFields(log.Fields{"host": ListenHTTPHost, "error": err, "type": consts.NetworkError}).Debug("cannot listen at host")
	}

	go func() {
		srv := &http.Server{Handler: route}
		err = srv.Serve(l)
		if err != nil {
			log.WithFields(log.Fields{"host": ListenHTTPHost, "error": err, "type": consts.NetworkError}).Error("serving http at host")
			panic(err)
		}
	}()
}
