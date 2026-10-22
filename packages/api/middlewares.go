/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/metrics"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/utils"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func authRequire(next func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		client := getClient(r)
		if client != nil && client.KeyID != 0 {
			next(w, r)
			return
		}

		logger := getLogger(r)
		logger.WithFields(log.Fields{"type": consts.EmptyObject}).Debug("wallet is empty")
		errorResponse(w, errUnauthorized)
	}
}

func loggerFromRequest(r *http.Request) *log.Entry {
	return log.WithFields(log.Fields{
		"headers":  utils.LogHeaders(r.Header),
		"path":     r.URL.Path,
		"protocol": r.Proto,
		"remote":   r.RemoteAddr,
	})
}
func loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := loggerFromRequest(r)
		logger.Info("received http request")
		r = setLogger(r, logger)
		next.ServeHTTP(w, r)
	})
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger := getLogger(r)
				logger.WithFields(log.Fields{
					"type":  consts.PanicRecoveredError,
					"error": err,
					"stack": string(debug.Stack()),
				}).Debug("panic recovered error")
				fmt.Println("API Recovered", fmt.Sprintf("%s: %s", err, debug.Stack()))
				errorResponse(w, errRecovered)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func nodeStateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reason errType
		switch node.NodePauseType() {
		case node.NoPause:
			next.ServeHTTP(w, r)
			return
		case node.PauseTypeUpdatingBlockchain:
			var s sqldb.BlockChain
			s.GetMaxBlock()
			reason = errUpdating.Errorf(s.ID)
			break
		case node.PauseTypeStopingNetwork:
			reason = errStopping
			break
		}
		errorResponse(w, reason)
	})
}

func tokenMiddleware(next http.Handler) http.Handler {
	const authHeader = "AUTHORIZATION"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//token, err := RefreshToken(r.Header.Get(authHeader))
		token, err := parseJWTToken(r.Header.Get(authHeader))
		if err != nil {
			logger := getLogger(r)
			logger.WithFields(log.Fields{"type": consts.JWTError, "error": err}).Warning("starting session")
		}
		if token != nil && token.Valid {
			r = setToken(r, token)
		}
		next.ServeHTTP(w, r)
	})
}

func (m Mode) clientMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := getToken(r)
		var client *Client
		if token != nil { // get client from token
			var err error
			if client, err = getClientFromToken(token, m.EcosystemGetter); err != nil {
				errorResponse(w, err)
				return
			}
		}
		if client == nil {
			// create client with default ecosystem
			client = &Client{EcosystemID: 1}
		}
		r = setClient(r, client)
		next.ServeHTTP(w, r)
	})
}

// metricsMiddleware times each request by the template of its route, so the routes and not the
// paths requested label the metric
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		next.ServeHTTP(w, r)
		route := "unmatched"
		if cur := mux.CurrentRoute(r); cur != nil {
			if tpl, err := cur.GetPathTemplate(); err == nil {
				route = tpl
			}
		}
		metrics.APIRequest.WithLabelValues("rest", r.Method, route).Observe(metrics.Since(startTime))
	})
}
