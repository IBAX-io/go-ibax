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
	"github.com/IBAX-io/go-ibax/packages/service"
	"github.com/IBAX-io/go-ibax/packages/statsd"
			return
		}

		logger := getLogger(r)
		logger.WithFields(log.Fields{"type": consts.EmptyObject}).Debug("wallet is empty")
		errorResponse(w, errUnauthorized)
	}
}

func loggerFromRequest(r *http.Request) *log.Entry {
	return log.WithFields(log.Fields{
		"headers":  r.Header,
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
				}).Error("panic recovered error")
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
		switch service.NodePauseType() {
		case service.NoPause:
			next.ServeHTTP(w, r)
			return
		case service.PauseTypeUpdatingBlockchain:
			reason = errUpdating
			break
		case service.PauseTypeStopingNetwork:
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
			if client, err = getClientFromToken(token, m.EcosysNameGetter); err != nil {
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

func statsdMiddleware(next http.Handler) http.Handler {
	const v = 1.0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := mux.CurrentRoute(r)

		counterName := statsd.APIRouteCounterName(r.Method, route.GetName())
		statsd.Client.Inc(counterName+statsd.Count, 1, v)
		startTime := time.Now()

		defer func() {
			statsd.Client.TimingDuration(counterName+statsd.Time, time.Since(startTime), v)
		}()

		next.ServeHTTP(w, r)
	})
}
