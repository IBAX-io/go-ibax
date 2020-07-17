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
	"time"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/crypto"
	"github.com/IBAX-io/go-ibax/packages/types"

	"github.com/dgrijalva/jwt-go"
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
	jwt.StandardClaims
}

func generateJWTToken(claims JWTClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func generateNewJWTToken(claims JWTClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ret, err := token.SignedString(jwtSecret)
	if err == nil {
		gr := GRefreshClaims{
}

func generateRefreshJWTToken(claims JWTClaims, h string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ret, err := token.SignedString(jwtSecret)
	if err == nil {
		gr := GRefreshClaims{
			Header:           h,
			Refresh:          ret,
			ExpiresAt:        claims.ExpiresAt,
			RefreshExpiresAt: claims.ExpiresAt,
		}
		gr.RefreshClaims()
	}
	return ret, err
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

	return jwt.ParseWithClaims(header, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(jwtSecret), nil
	})
}

func RefreshToken(header string) (*jwt.Token, error) {
	if len(header) == 0 {
		return nil, nil
	}
	if strings.HasPrefix(header, jwtPrefix) {
		header = header[len(jwtPrefix):]
	} else {
		return nil, errJWTAuthValue
	}

	gr := GRefreshClaims{}
	f := gr.ContainsClaims(header)
	if f {

		token, err := jwt.ParseWithClaims(gr.Refresh, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
			}

			return []byte(jwtSecret), nil
		})
		if err != nil {
			return nil, err
		}

		if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
			dnw := time.Now().Add(time.Second * time.Duration(jwtrefeshExpire)).Unix()
			if dnw > claims.StandardClaims.ExpiresAt {
				claims.StandardClaims.ExpiresAt = dnw

				tk, err := generateRefreshJWTToken(*claims, header)
				if err != nil {
					return nil, err
				}

				return jwt.ParseWithClaims(tk, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
					if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
						return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
					}

					return []byte(jwtSecret), nil
				})
			}
			return token, nil
		}
		gd := GRefreshClaims{
			Header: header,
		}
		gd.DeleteClaims()
		return token, err
	}

	token, err := jwt.ParseWithClaims(header, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(jwtSecret), nil
	})
	if err != nil {
		return nil, err
	}

	return token, err
}

func getClientFromToken(token *jwt.Token, ecosysNameService types.EcosystemNameGetter) (*Client, error) {
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
	result.ExpiresAt = claims.ExpiresAt
}
