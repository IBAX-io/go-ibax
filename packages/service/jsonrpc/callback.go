/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	log "github.com/sirupsen/logrus"
	"reflect"
	"runtime"
	"unicode"
)

var (
	errorType     = reflect.TypeOf((*Error)(nil)).Elem()
	contextType   = reflect.TypeOf((*RequestContext)(nil)).Elem()
	authType      = reflect.TypeOf((*Auth)(nil)).Elem()
	notSingleType = reflect.TypeOf((*NotSingle)(nil)).Elem()
)

type callback struct {
	fn        reflect.Value  // the function
	recv      reflect.Value  // receiver object of function
	argTypes  []reflect.Type // params types
	hasCtx    bool           // method's first argument is a RequestContext (not included in argTypes)
	errIndex  int            // err return index, of 0 when method cannot return error
	hasAuth   bool           // has auth
	notSingle bool
}

func (c *callback) getArgsTypes() {
	fntype := c.fn.Type()
	// Skip receiver and context.Context parameter (if present).
	firstArg := 0
	if c.recv.IsValid() {
		firstArg++
	}
	if fntype.NumIn() > firstArg && fntype.In(firstArg) == contextType {
		c.hasCtx = true
		firstArg++
	}
	if fntype.NumIn() > firstArg && fntype.In(firstArg) == authType {
		c.hasAuth = true
		firstArg++
	}
	if fntype.NumIn() > firstArg && fntype.In(firstArg) == notSingleType {
		c.notSingle = true
		firstArg++
	}
	// Add all remaining parameters.
	c.argTypes = make([]reflect.Type, fntype.NumIn()-firstArg)
	for i := firstArg; i < fntype.NumIn(); i++ {
		c.argTypes[i-firstArg] = fntype.In(i)
	}
}

func (c *callback) call(ctx RequestContext, m Mode, args []reflect.Value) (result any, errRes *Error) {
	// Create the argument slice.
	values := make([]reflect.Value, 0, 4+len(args))
	if c.recv.IsValid() {
		values = append(values, c.recv)
	}
	if c.hasCtx {
		values = append(values, reflect.ValueOf(ctx))
	}
	if c.hasAuth {
		auth := Auth{m}
		values = append(values, reflect.ValueOf(auth))
	}
	if c.notSingle {
		values = append(values, reflect.ValueOf(NotSingle{}))
	}
	values = append(values, args...)

	// Catch panic.
	defer func() {
		if err := recover(); err != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			req := ctx.HTTPRequest()
			logger := getLogger(req)
			logger.WithFields(log.Fields{"type": consts.JsonRpcError, "error": err, "buf": string(buf)}).Error("RPC method " + req.Method + " crashed")
			errRes = InternalError("method handler crashed")
		}
	}()
	// call func.
	results := c.fn.Call(values)
	if len(results) == 0 {
		return nil, nil
	}
	if c.errIndex > 0 && !results[c.errIndex].IsNil() {
		// Method has returned non-nil error value.
		err := results[c.errIndex].Interface().(*Error)
		return reflect.Value{}, err
	}
	if results[0].IsNil() {
		return nil, nil
	}
	return results[0].Interface(), nil
}

func suitableCallbacks(receiver reflect.Value) map[string]*callback {
	typ := receiver.Type()
	callbacks := make(map[string]*callback)
	for m := 0; m < typ.NumMethod(); m++ {
		method := typ.Method(m)
		if method.PkgPath != "" {
			continue // method not exported
		}
		cb := newCallback(receiver, method.Func)
		if cb == nil {
			log.WithFields(log.Fields{"err": "[json-rpv]method invalid", "method name": formatName(method.Name)})
			continue // function invalid
		}
		name := formatName(method.Name)
		callbacks[name] = cb
	}
	return callbacks
}

func newCallback(receiver, fn reflect.Value) *callback {
	ftype := fn.Type()
	c := &callback{fn: fn, recv: receiver, errIndex: -1}
	// Determine parameter types. They must all be exported or builtin types.
	c.getArgsTypes()

	// Verify return types. The function must return at most one error
	// and/or one other non-error value.
	outs := make([]reflect.Type, ftype.NumOut())
	for i := 0; i < ftype.NumOut(); i++ {
		outs[i] = ftype.Out(i)
	}
	if len(outs) > 2 {
		return nil
	}

	switch {
	//
	case len(outs) == 1 && isErrorType(outs[0]):
		c.errIndex = 0
	// If an error is returned, it must be the last returned value.
	case len(outs) == 2:
		if isErrorType(outs[0]) || !isErrorType(outs[1]) {
			return nil
		}
		c.errIndex = 1
	}
	return c
}

func isErrorType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	//return t.Implements(errorType) //t type is interface
	return t == errorType //t type is struct
}

func formatName(name string) string {
	ret := []rune(name)
	if len(ret) > 0 {
		ret[0] = unicode.ToLower(ret[0])
	}
	return string(ret)
}
