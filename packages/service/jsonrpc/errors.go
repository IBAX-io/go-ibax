package jsonrpc

import (
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/consts"
)

type ErrorCode int

type Error struct {
	Code    ErrorCode      `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

const (
	ErrCodeDefault             = -32000
	ErrCodeInvalidInput        = -32001
	ErrCodeResourceNotFound    = -32002
	ErrCodeResourceUnavailable = -32003
	ErrCodeTransactionRejected = -32004
	ErrCodeMethodNotSupported  = -32005
	ErrCodeLimitExceeded       = -32006
	ErrCodeParseError          = -32007
	ErrCodeInvalidRequest      = -32008
	ErrCodeMethodNotFound      = -32009
	ErrCodeInvalidParams       = -32010
	ErrCodeInternalError       = -32011
	ErrCodeNotFound            = -32012
	ErrCodeUnknownUID          = -32013
	ErrCodeUnauthorized        = -32014
	ErrCodeParamsInvalid       = -32015
)

const (
	paramsEmpty   = "params can not be empty"
	invalidParams = "params %s invalid"
)

func (e *Error) Error() string {
	return e.Message
}

func NewError(code ErrorCode, message string, data ...map[string]any) *Error {
	e := Error{
		Code:    code,
		Message: message,
	}

	if len(data) > 0 {
		e.Data = data[0]
	}

	return &e
}

func DefaultError(message string) *Error {
	return &Error{
		Code:    ErrCodeDefault,
		Message: message,
	}
}

func ParseError(message string) *Error {
	return &Error{
		Code:    ErrCodeParseError,
		Message: message,
	}
}

func InvalidRequest(message string, data ...map[string]any) *Error {
	return NewError(ErrCodeInvalidRequest, message, data...)
}

func MethodNotFound(method string, data ...map[string]any) *Error {
	message := fmt.Sprintf("The method %s does not exist/is not available", method)
	return NewError(ErrCodeMethodNotFound, message, data...)
}

func InvalidParamsError(message string, data ...map[string]any) *Error {
	return NewError(ErrCodeInvalidParams, message, data...)
}

func InternalError(message string, data ...map[string]any) *Error {
	return NewError(ErrCodeInternalError, message, data...)
}

func InvalidInput(message string, data ...map[string]any) *Error {
	return NewError(ErrCodeInvalidInput, message, data...)
}

func ResourceNotFound(message string, data ...map[string]any) *Error {
	return NewError(ErrCodeResourceNotFound, message, data...)
}

func ResourceUnavailable(message string, data ...map[string]any) *Error {
	return NewError(ErrCodeResourceUnavailable, message, data...)
}

func TransactionRejected(message string, data ...map[string]any) *Error {
	return NewError(ErrCodeTransactionRejected, message, data...)
}

func MethodNotSupported(method string, data ...map[string]any) *Error {
	message := fmt.Sprintf("method not supported %s", method)
	return NewError(ErrCodeMethodNotSupported, message, data...)
}

func LimitExceeded(message string, data ...map[string]any) *Error {
	return NewError(ErrCodeLimitExceeded, message, data...)
}

func NotFoundError() *Error {
	return NewError(ErrCodeNotFound, consts.NotFound)
}

func UnauthorizedError() *Error {
	return NewError(ErrCodeUnauthorized, "Unauthorized")
}

func UnUnknownUIDError() *Error {
	return NewError(ErrCodeUnknownUID, "Unknown uid")
}
