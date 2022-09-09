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

	"github.com/golang-jwt/jwt/v4"
	log "github.com/sirupsen/logrus"
)

const jwtUIDExpire = time.Second * 5

type getUIDResult struct {
	UID         string `json:"uid,omitempty"`
	Token       string `json:"token,omitempty"`
	Expire      string `json:"expire,omitempty"`
	EcosystemID string `json:"ecosystem_id,omitempty"`
	KeyID       string `json:"key_id,omitempty"`
	Address     string `json:"address,omitempty"`
	NetworkID   string `json:"network_id,omitempty"`
	Cryptoer    string `json:"cryptoer"`
	Hasher      string `json:"hasher"`
}

func getUIDHandler(w http.ResponseWriter, r *http.Request) {
	result := new(getUIDResult)
	result.NetworkID = converter.Int64ToStr(conf.Config.LocalConf.NetworkID)
	token := getToken(r)
	result.Cryptoer, result.Hasher = conf.Config.CryptoSettings.Cryptoer, conf.Config.CryptoSettings.Hasher
	if token != nil {
		if claims, ok := token.Claims.(*JWTClaims); ok && len(claims.KeyID) > 0 {
			result.EcosystemID = claims.EcosystemID
			result.Expire = claims.ExpiresAt.Sub(time.Now()).String()
			result.KeyID = claims.KeyID
			result.Address = converter.AddressToString(converter.StrToInt64(claims.KeyID))
			jsonResponse(w, result)
			return
		}
	}

	result.UID = converter.Int64ToStr(rand.New(rand.NewSource(time.Now().Unix())).Int63())
	claims := JWTClaims{
		UID:         result.UID,
		EcosystemID: "1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(jwtUIDExpire)},
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
