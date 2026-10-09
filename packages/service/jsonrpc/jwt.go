/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"errors"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/login"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/golang-jwt/jwt/v4"
	"strings"
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

func getClientFromToken(token *jwt.Token, ecosysNameService types.EcosystemGetter) (*UserClient, error) {
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, nil
	}
	if len(claims.KeyID) == 0 {
		return nil, nil
	}

	client := &UserClient{
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
