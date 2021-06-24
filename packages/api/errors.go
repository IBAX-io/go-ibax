/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	defaultStatus        = http.StatusBadRequest
	ErrEcosystemNotFound = errors.New("Ecosystem not found")
	errContract          = errType{"E_CONTRACT", "There is not %s contract", http.StatusNotFound}
	errDBNil             = errType{"E_DBNIL", "DB is nil", defaultStatus}
	errDeletedKey        = errType{"E_DELETEDKEY", "The key is deleted", http.StatusForbidden}
	errEcosystem         = errType{"E_ECOSYSTEM", "Ecosystem %d doesn't exist", defaultStatus}
	errEmptyPublic       = errType{"E_EMPTYPUBLIC", "Public key is undefined", http.StatusBadRequest}
	errKeyNotFound       = errType{"E_KEYNOTFOUND", "Key has not been found", http.StatusNotFound}
	errEmptySign         = errType{"E_EMPTYSIGN", "Signature is undefined", defaultStatus}
	errHashWrong         = errType{"E_HASHWRONG", "Hash is incorrect", http.StatusBadRequest}
	errHashNotFound      = errType{"E_HASHNOTFOUND", "Hash has not been found", defaultStatus}
	errHeavyPage         = errType{"E_HEAVYPAGE", "This page is heavy", defaultStatus}
	errInstalled         = errType{"E_INSTALLED", "Chain is already installed", defaultStatus}
	errInvalidWallet     = errType{"E_INVALIDWALLET", "Wallet %s is not valid", http.StatusBadRequest}
	errLimitForsign      = errType{"E_LIMITFORSIGN", "Length of forsign is too big (%d)", defaultStatus}
	errLimitTxSize       = errType{"E_LIMITTXSIZE", "The size of tx is too big (%d)", defaultStatus}
	errNotFound          = errType{"E_NOTFOUND", "Page not found", http.StatusNotFound}
	errNotFoundRecord    = errType{"E_NOTFOUND", "Record not found", http.StatusNotFound}
	errParamNotFound     = errType{"E_PARAMNOTFOUND", "Parameter %s has not been found", http.StatusNotFound}
	errPermission        = errType{"E_PERMISSION", "Permission denied", http.StatusUnauthorized}
	errQuery             = errType{"E_QUERY", "DB query is wrong", http.StatusInternalServerError}
	errRecovered         = errType{"E_RECOVERED", "API recovered", http.StatusInternalServerError}
	errServer            = errType{"E_SERVER", "Server error", defaultStatus}
	errSignature         = errType{"E_SIGNATURE", "Signature is incorrect", http.StatusBadRequest}
	errUnknownSign       = errType{"E_UNKNOWNSIGN", "Unknown signature", defaultStatus}
	errStateLogin        = errType{"E_STATELOGIN", "%d is not a membership of ecosystem %d", http.StatusForbidden}
	errTableNotFound     = errType{"E_TABLENOTFOUND", "Table %s has not been found", http.StatusNotFound}
	errToken             = errType{"E_TOKEN", "Token is not valid", defaultStatus}
	errTokenExpired      = errType{"E_TOKENEXPIRED", "Token is expired by %s", http.StatusUnauthorized}
	errUnauthorized      = errType{"E_UNAUTHORIZED", "Unauthorized", http.StatusUnauthorized}
	errUndefineval       = errType{"E_UNDEFINEVAL", "Value %s is undefined", defaultStatus}
	errUnknownUID        = errType{"E_UNKNOWNUID", "Unknown uid", defaultStatus}
	errOBS               = errType{"E_OBS", "Virtual Dedicated Ecosystem %d doesn't exist", defaultStatus}
	errOBSCreated        = errType{"E_OBSCREATED", "Virtual Dedicated Ecosystem is already created", http.StatusBadRequest}
	Err     string `json:"error"`
	Message string `json:"msg"`
	Status  int    `json:"-"`
}

func (et errType) Error() string {
	return et.Err
}

func (et errType) Errorf(v ...interface{}) errType {
	et.Message = fmt.Sprintf(et.Message, v...)
	return et
}
