/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestPublicAddress(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8":          true,
		"2001:4860::8888":  true,
		"127.0.0.1":        false,
		"::1":              false,
		"10.1.2.3":         false,
		"172.16.0.1":       false,
		"192.168.1.1":      false,
		"169.254.169.254":  false,
		"fe80::1":          false,
		"fc00::1":          false,
		"100.64.0.1":       false,
		"0.0.0.0":          false,
		"::":               false,
		"224.0.0.1":        false,
		"::ffff:127.0.0.1": false,
		"::ffff:8.8.8.8":   true,
	}
	for addr, want := range cases {
		if got := publicAddress(netip.MustParseAddr(addr)); got != want {
			t.Errorf("publicAddress(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestContractHTTPClientRejectsLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	_, err := contractHTTPClient.Get(srv.URL)
	if !errors.Is(err, errHTTPAddress) {
		t.Fatalf("err = %v, want %v", err, errHTTPAddress)
	}
}

func TestContractHTTPClientChecksRedirects(t *testing.T) {
	allowAll := func(netip.Addr) bool { return true }
	client := newContractHTTPClient(allowAll)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/hosts", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := client.Get(srv.URL); !errors.Is(err, errHTTPScheme) {
		t.Fatalf("scheme redirect: err = %v, want %v", err, errHTTPScheme)
	}

	loop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
	}))
	defer loop.Close()
	if _, err := client.Get(loop.URL); !errors.Is(err, errHTTPRedirects) {
		t.Fatalf("redirect loop: err = %v, want %v", err, errHTTPRedirects)
	}
}

func TestHTTPRequestRejectsScheme(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "file:///etc/hosts", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := httpRequest(req, nil); err == nil {
		t.Fatal("httpRequest accepted a file URL")
	}
}

func TestReadHTTPBodyLimit(t *testing.T) {
	if _, err := readHTTPBody(strings.NewReader(strings.Repeat("a", contractHTTPMaxBody))); err != nil {
		t.Fatalf("body at the limit: %v", err)
	}
	if _, err := readHTTPBody(strings.NewReader(strings.Repeat("a", contractHTTPMaxBody+1))); !errors.Is(err, errHTTPBodyTooLong) {
		t.Fatalf("body over the limit: err = %v, want %v", err, errHTTPBodyTooLong)
	}
}
