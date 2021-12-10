/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

type hashProvider int

const (
	_SHA256 hashProvider = iota
)

// getHMAC returns HMAC hash
func getHMAC(secret string, message string) ([]byte, error) {
	switch hmacProv {
	case _SHA256:
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(message))
		return mac.Sum(nil), nil
	default:
		return nil, ErrUnknownProvider
	}
}

// GetHMACWithTimestamp allows add timestamp
func GetHMACWithTimestamp(secret string, message string, timestamp string) ([]byte, error) {
	switch hmacProv {
	case _SHA256:
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(message))
		mac.Write([]byte(timestamp))
		return mac.Sum(nil), nil
	default:
		return nil, ErrUnknownProvider
	}
}

// _Hash returns hash of passed bytes
func (h *SHA256) _Hash(msg []byte) []byte {
	switch hashProv {
	case _SHA256:
		return hashSHA256(msg)
	default:
		return nil
	}
}

func hashSHA256(msg []byte) []byte {
	hash := sha256.Sum256(msg)
	return hash[:]
}

func HashHex(input []byte) (string, error) {
	return hex.EncodeToString(getHasher().hash(input)), nil
}
