/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

const (
	contractHTTPTimeout      = 30 * time.Second
	contractHTTPMaxRedirects = 10
	contractHTTPMaxBody      = 10 << 20
)

var (
	errHTTPScheme      = errors.New("only http and https URLs are allowed")
	errHTTPAddress     = errors.New("the destination address is not allowed")
	errHTTPRedirects   = errors.New("too many redirects")
	errHTTPBodyTooLong = fmt.Errorf("the response body exceeds %d bytes", contractHTTPMaxBody)

	sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

	contractHTTPClient = newContractHTTPClient(publicAddress)
)

// publicAddress reports whether ip is a globally routable unicast address.
func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && ip.IsGlobalUnicast() && !ip.IsPrivate() && !sharedAddressSpace.Contains(ip)
}

// newContractHTTPClient returns the client used by HTTPRequest and HTTPPostJSON.
// Every connection, including the ones made for redirects, is checked against allow
// after the host name is resolved.
func newContractHTTPClient(allow func(netip.Addr) bool) *http.Client {
	dialer := &net.Dialer{
		Timeout: contractHTTPTimeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			addrPort, err := netip.ParseAddrPort(address)
			if err != nil || !allow(addrPort.Addr()) {
				return errHTTPAddress
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: contractHTTPTimeout,
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: contractHTTPTimeout,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= contractHTTPMaxRedirects {
				return errHTTPRedirects
			}
			return checkHTTPScheme(req)
		},
	}
}

func checkHTTPScheme(req *http.Request) error {
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return errHTTPScheme
	}
	return nil
}

func readHTTPBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, contractHTTPMaxBody+1))
	if err != nil {
		return nil, err
	}
	if len(data) > contractHTTPMaxBody {
		return nil, errHTTPBodyTooLong
	}
	return data, nil
}
