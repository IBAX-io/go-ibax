/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package utils

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestToSnakeCase(t *testing.T) {
	cases := []struct {
		arg, expected string
	}{
		{"Contains", "contains"},
		{"AddressToId", "address_to_id"},
		{"HMac", "h_mac"},
		{"JSONEncode", "json_encode"},
		{"Hash", "hash"},
		{"PubToID", "pub_to_id"},
	}
	for _, tt := range cases {
		assert.Equal(t, tt.expected, ToSnakeCase(tt.arg))
	}
}

// fakeNTP answers NTP requests on a local UDP port with the time shifted by offset
func fakeNTP(t *testing.T, offset time.Duration) string {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		request := make([]byte, 48)
		for {
			_, from, err := conn.ReadFromUDP(request)
			if err != nil {
				return
			}
			nanosec := uint64(time.Now().Add(offset).Sub(time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)))
			reply := make([]byte, 48)
			binary.BigEndian.PutUint32(reply[40:], uint32(nanosec/1e9))
			binary.BigEndian.PutUint32(reply[44:], uint32((nanosec%1e9)<<32/1e9))
			conn.WriteToUDP(reply, from)
		}
	}()
	return conn.LocalAddr().String()
}

func TestSntpDrift(t *testing.T) {
	for _, offset := range []time.Duration{0, 3 * time.Second, -3 * time.Second} {
		drift, err := sntpDrift(fakeNTP(t, offset), ntpChecks)
		if err != nil {
			t.Fatal(err)
		}
		// The local clock is behind a server ahead of it: the drift is the opposite of the offset
		assert.InDelta(t, float64(-offset), float64(drift), float64(100*time.Millisecond), "offset %v", offset)
	}
}
