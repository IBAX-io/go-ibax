/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/login"
)

type getUIDResult = login.UIDResult

func getUIDHandler(w http.ResponseWriter, r *http.Request) {
	result, err := login.UID(tokenClaims(r))
	if err != nil {
		loginErrorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}

// tokenClaims are the claims of the valid token the request carries, nil without one
func tokenClaims(r *http.Request) *JWTClaims {
	if token := getToken(r); token != nil {
		if claims, ok := token.Claims.(*JWTClaims); ok {
			return claims
		}
	}
	return nil
}

// loginErrorResponse answers a refused login with its code, any other error as the API does
func loginErrorResponse(w http.ResponseWriter, err error) {
	if refusal, ok := err.(*login.Error); ok {
		errorResponse(w, errType{Err: refusal.Code, Message: refusal.Msg, Status: refusal.Status})
		return
	}
	errorResponse(w, err)
}
