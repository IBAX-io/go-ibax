/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"errors"
	"net/http"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/login"
	log "github.com/sirupsen/logrus"
)

type AuthStatusResponse struct {
	IsActive  bool  `json:"active"`
	ExpiresAt int64 `json:"exp,omitempty"`
}

type Auth struct {
	Mode
}

func authRequire(r *http.Request) *Error {
	client := getClient(r)
	if client != nil && client.KeyID != 0 {
		return nil
	}

	logger := getLogger(r)
	logger.WithFields(log.Fields{"type": consts.EmptyObject}).Debug("wallet is empty")
	return UnauthorizedError()
}

type authApi struct {
	mode Mode
}

func newAuthApi(mode Mode) *authApi {
	a := &authApi{
		mode: mode,
	}
	return a
}

func (a *authApi) GetAuthStatus(ctx RequestContext) (*AuthStatusResponse, *Error) {
	result := new(AuthStatusResponse)

	r := ctx.HTTPRequest()
	token := getToken(r)
	if token == nil {
		return result, nil
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return result, nil
	}

	result.IsActive = true
	result.ExpiresAt = claims.ExpiresAt.Unix()
	return result, nil
}

type GetUIDResult = login.UIDResult

// tokenClaims are the claims of the valid token the request carries, nil without one
func tokenClaims(r *http.Request) *JWTClaims {
	if token := getToken(r); token != nil {
		if claims, ok := token.Claims.(*JWTClaims); ok {
			return claims
		}
	}
	return nil
}

// loginError is a refused login as a JSON-RPC error, its data the code of the refusal
func loginError(err error) *Error {
	refusal, ok := err.(*login.Error)
	if !ok {
		return DefaultError(err.Error())
	}
	data := map[string]any{"error": refusal.Code}
	switch {
	case refusal == login.ErrUnknownUID:
		return NewError(ErrCodeUnknownUID, refusal.Msg, data)
	case refusal.Status == http.StatusUnauthorized || refusal.Status == http.StatusForbidden:
		return NewError(ErrCodeUnauthorized, refusal.Msg, data)
	case refusal.Status == http.StatusNotFound:
		return NewError(ErrCodeNotFound, refusal.Msg, data)
	case refusal.Status == http.StatusServiceUnavailable:
		return ResourceUnavailable(refusal.Msg, data)
	}
	return InvalidParamsError(refusal.Msg, data)
}

func (a *authApi) GetUid(ctx RequestContext) (*GetUIDResult, *Error) {
	result, err := login.UID(tokenClaims(ctx.HTTPRequest()))
	if err != nil {
		return nil, loginError(err)
	}
	return result, nil
}

type loginForm struct {
	EcosystemID int64          `json:"ecosystem_id"`
	Expire      int64          `json:"expire"`
	PublicKey   hexValue `json:"public_key"`
	KeyID       string         `json:"key_id"`
	Signature   hexValue       `json:"signature"`
	RoleID      int64          `json:"role_id"`
	Guest       bool           `json:"guest"`
}

func (f *loginForm) Validate(r *http.Request) error {
	if f == nil {
		return errors.New(paramsEmpty)
	}
	return nil
}

type LoginResult = login.Result

func (a authApi) Login(ctx RequestContext, form *loginForm) (*LoginResult, *Error) {
	r := ctx.HTTPRequest()
	claims := tokenClaims(r)
	if err := form.Validate(r); err != nil {
		login.Discard(claims)
		return nil, InvalidParamsError(err.Error())
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
		return nil, loginError(err)
	}
	return result, nil
}
