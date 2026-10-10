/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/login"
)

type loginForm struct {
	EcosystemID int64    `schema:"ecosystem"`
	Expire      int64    `schema:"expire"`
	PublicKey   hexValue `schema:"pubkey"`
	KeyID       string   `schema:"key_id"`
	Signature   hexValue `schema:"signature"`
	RoleID      int64    `schema:"role_id"`
	Guest       bool     `schema:"guest"`
}

func (f *loginForm) Validate(r *http.Request) error {
	return nil
}

type loginResult = login.Result

func (m Mode) loginHandler(w http.ResponseWriter, r *http.Request) {
	claims := tokenClaims(r)
	form := new(loginForm)
	if err := parseForm(r, form); err != nil {
		login.Discard(claims)
		errorResponse(w, err, http.StatusBadRequest)
		return
	}
	result, err := login.Login(login.Request{
		Token:     claims,
		Ecosystem: form.EcosystemID,
		Expire:    form.Expire,
		PublicKey: form.PublicKey.Bytes(),
		KeyID:     form.KeyID,
		Signature: form.Signature.Bytes(),
		RoleID:    form.RoleID,
		Guest:     form.Guest,
	}, getLogger(r))
	if err != nil {
		loginErrorResponse(w, err)
		return
	}
	jsonResponse(w, result)
}
