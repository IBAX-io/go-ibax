/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/login"
	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/golang-jwt/jwt/v4"
)

var (
	jwtPrefix = "Bearer "

	errJWTAuthValue      = errors.New("wrong authorization value")
	errEcosystemNotFound = errors.New("ecosystem not found")
)

// JWTClaims is storing jwt claims
type JWTClaims = login.Claims

func parseJWTToken(header string) (*jwt.Token, error) {
	if len(header) == 0 {
		return nil, nil
	}

	if strings.HasPrefix(header, jwtPrefix) {
		header = header[len(jwtPrefix):]
	} else {
		return nil, errJWTAuthValue
	}

	return login.Parse(header)
}

func getClientFromToken(token *jwt.Token, ecosysNameService types.EcosystemGetter) (*Client, error) {
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, nil
	}
	if len(claims.KeyID) == 0 {
		return nil, nil
	}

	client := &Client{
		EcosystemID: converter.StrToInt64(claims.EcosystemID),
		KeyID:       converter.StrToInt64(claims.KeyID),
		AccountID:   claims.AccountID,
		RoleID:      converter.StrToInt64(claims.RoleID),
	}

	sID := converter.StrToInt64(claims.EcosystemID)
	name, err := ecosysNameService.GetEcosystemName(sID)
	if err != nil {
		return nil, err
	}

	client.EcosystemName = name
	return client, nil
}

type authStatusResponse struct {
	IsActive  bool  `json:"active"`
	ExpiresAt int64 `json:"exp,omitempty"`
}

func getAuthStatus(w http.ResponseWriter, r *http.Request) {
	result := new(authStatusResponse)
	defer jsonResponse(w, result)

	token := getToken(r)
	if token == nil {
		return
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return
	}

	result.IsActive = true
	result.ExpiresAt = claims.ExpiresAt.Unix()
}
