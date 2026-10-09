/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package login

import (
	"fmt"
	"net/http"
)

// Error is a refused login, as the API answers it: {"error": Code, "msg": Msg} with Status
type Error struct {
	Code   string `json:"error"`
	Msg    string `json:"msg"`
	Status int    `json:"-"`
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Msg
}

var (
	// ErrUnknownUID refuses a login without a challenge this node issued and nobody used yet
	ErrUnknownUID = &Error{Code: "E_UNKNOWNUID", Msg: "Unknown uid", Status: http.StatusBadRequest}
	// ErrBusy refuses a challenge while MaxChallenges are open
	ErrBusy = &Error{Code: "E_BUSY", Msg: "Too many open login challenges, try again later", Status: http.StatusServiceUnavailable}
	// ErrExpire refuses a session lifetime out of range
	ErrExpire = &Error{Code: "E_EXPIRE", Msg: fmt.Sprintf("expire must be from 0 to %d seconds", SessionLifetime), Status: http.StatusBadRequest}
	// ErrEmptyPublic refuses a login of a key the chain does not know, without its public key
	ErrEmptyPublic = &Error{Code: "E_EMPTYPUBLIC", Msg: "Public key is undefined", Status: http.StatusBadRequest}
	// ErrDiffKey refuses a key_id that is not the address of the public key
	ErrDiffKey = &Error{Code: "E_DIFKEY", Msg: "key_id is not the address of the public key", Status: http.StatusBadRequest}
	// ErrNewUser refuses a login of a key not registered in the ecosystem: the key signs and sends
	// @1NewUser itself, then logs in again
	ErrNewUser = &Error{Code: "E_NEWUSER", Msg: "The key is not registered in the ecosystem: send @1NewUser signed with it, then log in again", Status: http.StatusUnauthorized}
	// ErrKeyNotFound refuses the guest login in an ecosystem without the guest account
	ErrKeyNotFound = &Error{Code: "E_KEYNOTFOUND", Msg: "Key has not been found", Status: http.StatusNotFound}
	// ErrDeletedKey refuses a deleted key
	ErrDeletedKey = &Error{Code: "E_DELETEDKEY", Msg: "The key is deleted", Status: http.StatusForbidden}
	// ErrCheckRole refuses a role the account is not a member of
	ErrCheckRole = &Error{Code: "E_CHECKROLE", Msg: "Access denied", Status: http.StatusForbidden}
)

// ErrSignature refuses a signature that does not verify; msg says why
func ErrSignature(msg string) *Error {
	return &Error{Code: "E_SIGNATURE", Msg: msg, Status: http.StatusBadRequest}
}

// ErrEcoNotOpen refuses a new key in an ecosystem that does not take new members
func ErrEcoNotOpen(ecosystem int64) *Error {
	return &Error{Code: "E_ECONOTOPEN", Msg: fmt.Sprintf("The ecosystem (%d) is not open and cannot be registered address", ecosystem), Status: http.StatusUnauthorized}
}
