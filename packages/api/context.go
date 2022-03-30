/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"context"
	"net/http"

	"github.com/golang-jwt/jwt/v4"
	log "github.com/sirupsen/logrus"
)

type contextKey int

const (
	contextKeyLogger contextKey = iota
	contextKeyToken
	contextKeyClient
)

func setContext(r *http.Request, key, value any) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), key, value))
}

func getContext(r *http.Request, key any) any {
	return r.Context().Value(key)
}

func setLogger(r *http.Request, log *log.Entry) *http.Request {
	return setContext(r, contextKeyLogger, log)
}

func getLogger(r *http.Request) *log.Entry {
	if v := getContext(r, contextKeyLogger); v != nil {
		return v.(*log.Entry)
	}
	return nil
}

func setToken(r *http.Request, token *jwt.Token) *http.Request {
	return setContext(r, contextKeyToken, token)
}

func getToken(r *http.Request) *jwt.Token {
	if v := getContext(r, contextKeyToken); v != nil {
		return v.(*jwt.Token)
	}
	return nil
}

func setClient(r *http.Request, client *Client) *http.Request {
	return setContext(r, contextKeyClient, client)
}

func getClient(r *http.Request) *Client {
	if v := getContext(r, contextKeyClient); v != nil {
		return v.(*Client)
	}
	return nil
}
