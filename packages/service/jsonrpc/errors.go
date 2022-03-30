package jsonrpc

import (
	"fmt"
)

type ErrorCode int

type Error struct {
	Code    ErrorCode      `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

const (
	ErrCodeParseError          = -32700
	ErrCodeInvalidRequest      = -32600
	ErrCodeMethodNotFound      = -32601
	ErrCodeInvalidParams       = -32602
	ErrCodeInternalError       = -32603
	ErrCodeInvalidInput        = -32000
	ErrCodeResourceNotFound    = -32001
	ErrCodeResourceUnavailable = -32002
	ErrCodeTransactionRejected = -32003
	ErrCodeMethodNotSupported  = -32004
	ErrCodeLimitExceeded       = -32005
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

func ParseError(message string) *Error {
	return &Error{
		Code:    ErrCodeParseError,
		Message: message,
	}
}

func InvalidRequest(message string, data ...map[string]any) *Error {
	return NewError(ErrCodeInvalidRequest, message, data...)
}

func MethodNotFound(request *Request, data ...map[string]any) *Error {
	message := fmt.Sprintf("The method %s does not exist/is not available", request.Method)
	return NewError(ErrCodeMethodNotFound, message, data...)
}

func InvalidParams(message string, data ...map[string]any) *Error {
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

func MethodNotSupported(request *Request, data ...map[string]any) *Error {
	message := fmt.Sprintf("method not supported %s", request.Method)
	return NewError(ErrCodeMethodNotSupported, message, data...)
}

func LimitExceeded(message string, data ...map[string]any) *Error {
	return NewError(ErrCodeLimitExceeded, message, data...)
}
