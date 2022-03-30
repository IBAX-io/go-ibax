/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/golang-jwt/jwt/v4"
)

var (
	jwtSecret       = []byte(crypto.RandSeq(15))
	jwtPrefix       = "Bearer "
	jwtExpire       = 28800 // By default, seconds
	jwtrefeshExpire = 600   // By default, seconds
	//jwtrefeshExpire = 10   // By default, seconds  test

	errJWTAuthValue      = errors.New("wrong authorization value")
	errEcosystemNotFound = errors.New("ecosystem not found")
)

// JWTClaims is storing jwt claims
type JWTClaims struct {
	UID         string `json:"uid,omitempty"`
	EcosystemID string `json:"ecosystem_id,omitempty"`
	KeyID       string `json:"key_id,omitempty"`
	AccountID   string `json:"account_id,omitempty"`
	RoleID      string `json:"role_id,omitempty"`
	IsMobile    bool   `json:"is_mobile,omitempty"`
	jwt.RegisteredClaims
}

func generateJWTToken(claims JWTClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func parseJWTToken(header string) (*jwt.Token, error) {
	if len(header) == 0 {
		return nil, nil
	}

	if strings.HasPrefix(header, jwtPrefix) {
		header = header[len(jwtPrefix):]
	} else {
		return nil, errJWTAuthValue
	}

	return jwt.ParseWithClaims(header, &JWTClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(jwtSecret), nil
	})
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
		IsMobile:    claims.IsMobile,
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
