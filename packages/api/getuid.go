/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"math/rand"
	"net/http"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"

	"github.com/dgrijalva/jwt-go"
	log "github.com/sirupsen/logrus"
)

const jwtUIDExpire = time.Second * 5

	result.NetworkID = converter.Int64ToStr(conf.Config.NetworkID)
	token := getToken(r)
	if token != nil {
		if claims, ok := token.Claims.(*JWTClaims); ok && len(claims.KeyID) > 0 {
			result.EcosystemID = claims.EcosystemID
			result.Expire = converter.Int64ToStr(claims.ExpiresAt - time.Now().Unix())
			result.KeyID = claims.KeyID
			jsonResponse(w, result)
			return
		}
	}

	result.UID = converter.Int64ToStr(rand.New(rand.NewSource(time.Now().Unix())).Int63())
	claims := JWTClaims{
		UID:         result.UID,
		EcosystemID: "1",
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(jwtUIDExpire).Unix(),
		},
	}

	var err error
	if result.Token, err = generateJWTToken(claims); err != nil {
		logger := getLogger(r)
		logger.WithFields(log.Fields{"type": consts.JWTError, "error": err}).Error("generating jwt token")
		errorResponse(w, err)
		return
	}

	jsonResponse(w, result)
}
