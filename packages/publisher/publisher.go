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
	jwt "github.com/dgrijalva/jwt-go"
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
	jwt.StandardClaims
}

// InitCentrifugo client
func InitCentrifugo(cfg conf.CentrifugoConfig) {
	config = cfg
	publisher = gocent.New(gocent.Config{
		Addr: cfg.URL,
		Key:  cfg.Key,
	})
}
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
