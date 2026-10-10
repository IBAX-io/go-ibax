/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package publisher

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/IBAX-io/go-ibax/packages/conf"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const account = "1234-5678-9012-3456-7890"

// fakeCentrifugo answers the server API as Centrifugo 6 does, and records the requests
func fakeCentrifugo(t *testing.T, requests *[]string) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*requests = append(*requests, r.URL.Path+" "+string(body))
		if r.Header.Get("X-API-Key") != "api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/info":
			w.Write([]byte(`{"result":{"nodes":[{"uid":"1","version":"6.9.7 OSS"}]}}`))
		case "/api/publish":
			w.Write([]byte(`{"result":{}}`))
		default:
			w.Write([]byte(`{"error":{"code":104,"message":"method not found"}}`))
		}
	}))
	t.Cleanup(server.Close)
	InitCentrifugo(conf.CentrifugoConfig{URL: server.URL, Key: "api-key", Secret: "secret"})
	t.Cleanup(func() { InitCentrifugo(conf.CentrifugoConfig{}) })
}

func TestConnectionToken(t *testing.T) {
	InitCentrifugo(conf.CentrifugoConfig{Secret: "secret"})
	defer InitCentrifugo(conf.CentrifugoConfig{})

	token, err := ConnectionToken(account, 60)
	require.NoError(t, err)
	var claims connectionClaims
	_, err = jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) { return []byte("secret"), nil })
	require.NoError(t, err)
	assert.Equal(t, account, claims.Subject)
	assert.Equal(t, []string{"client#" + account}, claims.Channels)
	assert.WithinDuration(t, time.Now().Add(time.Minute), claims.ExpiresAt.Time, 2*time.Second)
}

func TestWrite(t *testing.T) {
	var requests []string
	fakeCentrifugo(t, &requests)

	require.NoError(t, Write(account, `[{"count":1}]`))
	require.Len(t, requests, 1)
	assert.Equal(t, "/api/publish", requests[0][:len("/api/publish")])
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(requests[0][len("/api/publish "):]), &body))
	assert.Equal(t, map[string]any{"channel": "client#" + account, "data": []any{map[string]any{"count": float64(1)}}}, body)
}

func TestGetEndpoint(t *testing.T) {
	var requests []string
	fakeCentrifugo(t, &requests)

	endpoint, err := GetEndpoint()
	require.NoError(t, err)
	assert.Equal(t, Protocol, endpoint.Protocol)
	assert.Equal(t, "6", endpoint.Version)
	assert.Equal(t, "ws:", endpoint.URL[:3])

	config.Key = "wrong"
	_, err = GetEndpoint()
	assert.ErrorContains(t, err, "401")

	InitCentrifugo(conf.CentrifugoConfig{})
	_, err = GetEndpoint()
	assert.ErrorContains(t, err, "not configured")
}

func TestCallError(t *testing.T) {
	var requests []string
	fakeCentrifugo(t, &requests)

	assert.EqualError(t, call("nothing", struct{}{}, nil), "centrifugo nothing: 104 method not found")
}

func TestWebSocketURL(t *testing.T) {
	assert.Equal(t, "ws://127.0.0.1:8000", webSocketURL("http://127.0.0.1:8000"))
	assert.Equal(t, "wss://example.com", webSocketURL("https://example.com"))
	assert.Equal(t, "ws://example.com", webSocketURL("ws://example.com"))
}
