/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/service/node"
	"github.com/IBAX-io/go-ibax/packages/statsd"
	"github.com/didip/tollbooth"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"net/http"
	"runtime/debug"
	"time"
)

func clientMiddleware(next http.Handler, m Mode) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := getToken(r)
		var client *UserClient
		if token != nil { // get client from token
			var err error
			if client, err = getClientFromToken(token, m.EcosystemGetter); err != nil {
				WriteResponse(w, nil, nil, DefaultError(err.Error()))
				return
			}
		}
		if client == nil {
			// create client with default ecosystem
			client = &UserClient{EcosystemID: 1}
		}
		r = setClient(r, client)
		next.ServeHTTP(w, r)
	})
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
				}).Debug("panic recovered error")
				WriteResponse(w, nil, nil, InternalError("JSON RPC API recovered"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func nodeStateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch node.NodePauseType() {
		case node.NoPause:
			next.ServeHTTP(w, r)
			return
		case node.PauseTypeUpdatingBlockchain:
			WriteResponse(w, nil, nil, DefaultError("Node is updating blockchain"))
			break
		case node.PauseTypeStopingNetwork:
			WriteResponse(w, nil, nil, DefaultError("Network is stopping"))
			break
		}
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

func statsdMiddleware(next http.Handler) http.Handler {
	const v = 1.0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//request url
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

func limiterMiddleware(next http.Handler) http.Handler {
	//max ten requests per second
	limiter := tollbooth.NewLimiter(10, nil)
	return tollbooth.LimitHandler(limiter, next)
}
