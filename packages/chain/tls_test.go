/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package chain

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerTLSConfig(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.TLS = serverTLSConfig()
	srv.StartTLS()
	defer srv.Close()

	cases := []struct {
		name   string
		client *tls.Config
		ok     bool
	}{
		{"TLS 1.3", &tls.Config{MinVersion: tls.VersionTLS13}, true},
		{"TLS 1.2 AES-GCM", &tls.Config{MaxVersion: tls.VersionTLS12, CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}}, true},
		{"TLS 1.2 ChaCha20-Poly1305", &tls.Config{MaxVersion: tls.VersionTLS12, CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256}}, false},
		{"TLS 1.2 AES-CBC", &tls.Config{MaxVersion: tls.VersionTLS12, CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA, tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA}}, false},
		{"TLS 1.1", &tls.Config{MaxVersion: tls.VersionTLS11}, false},
	}
	for _, c := range cases {
		c.client.RootCAs = srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
		conn, err := tls.Dial("tcp", srv.Listener.Addr().String(), c.client)
		if err == nil {
			conn.Close()
		}
		if (err == nil) != c.ok {
			t.Errorf("%s: handshake error %v, want success %v", c.name, err, c.ok)
		}
	}
}
