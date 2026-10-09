/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package login

import (
	"fmt"

	"github.com/golang-jwt/jwt/v4"
)

// SessionLifetime is how long a session lasts, in seconds, unless the login asks for less
const SessionLifetime = 28800

var secret []byte

// Claims are what a token of the node's APIs says: a login challenge (UID) or a session (KeyID)
type Claims struct {
	UID         string `json:"uid,omitempty"`
	EcosystemID string `json:"ecosystem_id,omitempty"`
	KeyID       string `json:"key_id,omitempty"`
	AccountID   string `json:"account_id,omitempty"`
	RoleID      string `json:"role_id,omitempty"`
	jwt.RegisteredClaims
}

// InitSecret sets the HMAC-SHA256 key tokens are signed with
func InitSecret(key []byte) {
	if key == nil {
		panic("jwt secret invalid")
	}
	secret = key
}

// Sign makes a token of claims
func Sign(claims Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// Parse reads a token this node signed
func Parse(token string) (*jwt.Token, error) {
	return jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	})
}
