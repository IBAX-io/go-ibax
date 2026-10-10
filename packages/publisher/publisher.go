/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package publisher sends the notifications of accounts to their clients through Centrifugo. The
// node publishes with the server HTTP API of Centrifugo 5 and later, and signs the connection
// tokens of its sessions: a token subscribes its connection, on the server side, to the channel
// of its account, which is the only channel a client gets.
package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/consts"

	"github.com/golang-jwt/jwt/v4"
	log "github.com/sirupsen/logrus"
)

// Protocol is the protocol clients speak with Centrifugo
const Protocol = "centrifuge-json"

const centrifugoTimeout = 5 * time.Second

var (
	config conf.CentrifugoConfig
	client = &http.Client{Timeout: centrifugoTimeout}
)

// InitCentrifugo sets the server and the keys of Centrifugo
func InitCentrifugo(cfg conf.CentrifugoConfig) {
	config = cfg
}

// Channel is the channel of the notifications of the account: a user-limited channel, which only
// connections of the account can be subscribed to
func Channel(account string) string {
	return "client#" + account
}

type connectionClaims struct {
	Channels []string `json:"channels"`
	jwt.RegisteredClaims
}

// ConnectionToken is the token of a connection of the account to Centrifugo for expire seconds,
// subscribed to the account's channel
func ConnectionToken(account string, expire int64) (string, error) {
	claims := connectionClaims{
		Channels: []string{Channel(account)},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   account,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Second * time.Duration(expire))),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.Secret))
	if err != nil {
		log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("JWT centrifugo error")
		return "", err
	}
	return token, nil
}

// call runs the method of the server API of Centrifugo with the params, and decodes its result into
// result unless it is nil
func call(method string, params, result any) error {
	if config.URL == "" {
		return errors.New("centrifugo is not configured")
	}
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), centrifugoTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(config.URL, "/")+"/api/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", config.Key)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("centrifugo %s: %s %s", method, resp.Status, bytes.TrimSpace(data))
	}
	var reply struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Result json.RawMessage `json:"result"`
	}
	if err = json.Unmarshal(data, &reply); err != nil {
		return fmt.Errorf("centrifugo %s: %w", method, err)
	}
	if reply.Error != nil {
		return fmt.Errorf("centrifugo %s: %d %s", method, reply.Error.Code, reply.Error.Message)
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(reply.Result, result)
}

// Write publishes the data, a JSON value, to the channel of the account
func Write(account string, data string) error {
	return call("publish", map[string]any{"channel": Channel(account), "data": json.RawMessage(data)}, nil)
}

// Endpoint is where and how clients connect to Centrifugo
type Endpoint struct {
	// URL is the address of the server, with the WebSocket scheme
	URL      string `json:"url"`
	Protocol string `json:"protocol"`
	// Version is the major version of the server
	Version string `json:"version"`
}

// GetEndpoint is the endpoint of the Centrifugo server, which must answer
func GetEndpoint() (Endpoint, error) {
	var info struct {
		Nodes []struct {
			Version string `json:"version"`
		} `json:"nodes"`
	}
	if err := call("info", struct{}{}, &info); err != nil {
		return Endpoint{}, err
	}
	if len(info.Nodes) == 0 {
		return Endpoint{}, errors.New("centrifugo info: no nodes")
	}
	version, _, _ := strings.Cut(strings.TrimPrefix(info.Nodes[0].Version, "v"), ".")
	return Endpoint{URL: webSocketURL(config.URL), Protocol: Protocol, Version: version}, nil
}

// webSocketURL is the HTTP address with the WebSocket scheme
func webSocketURL(address string) string {
	if rest, ok := strings.CutPrefix(address, "http:"); ok {
		return "ws:" + rest
	}
	if rest, ok := strings.CutPrefix(address, "https:"); ok {
		return "wss:" + rest
	}
	return address
}
