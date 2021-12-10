/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package publisher

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"

	"github.com/centrifugal/gocent"
	"github.com/golang-jwt/jwt/v4"
	log "github.com/sirupsen/logrus"
)

type ClientsChannels struct {
	storage map[int64]string
	sync.RWMutex
}

func (cn *ClientsChannels) Set(id int64, s string) {
	cn.Lock()
	defer cn.Unlock()
	cn.storage[id] = s
}

func (cn *ClientsChannels) Get(id int64) string {
	cn.RLock()
	defer cn.RUnlock()
	return cn.storage[id]
}

var (
	clientsChannels   = ClientsChannels{storage: make(map[int64]string)}
	centrifugoTimeout = time.Second * 5
	publisher         *gocent.Client
	config            conf.CentrifugoConfig
)

type CentJWT struct {
	Sub string
	jwt.RegisteredClaims
}

// InitCentrifugo client
func InitCentrifugo(cfg conf.CentrifugoConfig) {
	config = cfg
	publisher = gocent.New(gocent.Config{
		Addr: cfg.URL,
		Key:  cfg.Key,
	})
}

func GetJWTCent(userID, expire int64) (string, string, error) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	centJWT := CentJWT{
		Sub: strconv.FormatInt(userID, 10),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(time.Second * time.Duration(expire))},
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, centJWT)
	result, err := token.SignedString([]byte(config.Secret))
	if err != nil {
		log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("JWT centrifugo error")
		return "", "", err
	}
	clientsChannels.Set(userID, result)
	return result, timestamp, nil
}

// Write is publishing data to server
func Write(account string, data string) error {
	ctx, cancel := context.WithTimeout(context.Background(), centrifugoTimeout)
	defer cancel()
	return publisher.Publish(ctx, "client"+account, []byte(data))
}

// GetStats returns Stats
func GetStats() (gocent.InfoResult, error) {
	if publisher == nil {
		return gocent.InfoResult{}, fmt.Errorf("publisher not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), centrifugoTimeout)
	defer cancel()
	return publisher.Info(ctx)
}
