/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

type service struct {
	callbacks map[string]*callback // registered handlers
}

type serviceRegistry struct {
	mu       sync.Mutex
	services map[string]service
	route    func(ctx RequestContext, request *Request)
	mode     Mode
	ctx      context.Context
	cancel   context.CancelFunc
	runWg    sync.WaitGroup
	t1       time.Time
}

func (r *serviceRegistry) registerName(namespace string, stct any) error {
	stctVal := GetStructValue(stct)
	if namespace == "" {
		return fmt.Errorf("no service name for type %s", stctVal.Type().String())
	}
	callbacks := suitableCallbacks(stctVal)
	if len(callbacks) == 0 {
		return fmt.Errorf("service %T doesn't have any suitable methods/subscriptions to expose", stct)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.services == nil {
		r.services = make(map[string]service)
	}
	svc, ok := r.services[namespace]
	if !ok {
		svc = service{
			callbacks: make(map[string]*callback),
		}
		r.services[namespace] = svc
	}
	for name, cb := range callbacks {
		svc.callbacks[name] = cb
	}
	return nil
}

// ServeHTTP json-rpc server
func (s *serviceRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.t1 = time.Now()
	ctx := context.WithValue(r.Context(), contextKeyHTTPRequest, r)
	ctx = context.WithValue(ctx, contextKeyHTTPResponseWriter, w)
	ctxer := func(raw json.RawMessage) {
		ctx = context.WithValue(ctx, contextKeyRawJSON, raw)
	}
	reqs, batch, err := getBatch(r.Body, ctxer)
	if err != nil {
		if err != io.EOF {
			WriteResponse(w, nil, nil, InvalidParamsError(err.Error()))
			return
		}
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	defer s.close(io.EOF)

	if batch {
		if len(reqs) == 0 {
			WriteResponse(w, nil, nil, InvalidInput("empty batch request"))
			return
		}
		s.runBatch(RequestContext{ctx}, reqs)
		return
	}

	result, e := s.run(RequestContext{ctx}, false, reqs)
	WriteResponse(w, reqs[0], result, e)
}

func (s *serviceRegistry) close(err error) {
	s.runWg.Wait()
	s.cancel()
}

func GetNamespace(name space) string {
	return string(name)
}

func (r *serviceRegistry) findCallback(method string) *callback {
	elem := strings.SplitN(method, namespaceSeparator, 2)
	if len(elem) != 2 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.services[elem[0]].callbacks[elem[1]]
}

func (s *serviceRegistry) run(ctx RequestContext, isBatch bool, reqs []*Request) (any, *Error) {
	req := reqs[0]
	cb := s.findCallback(req.Method)
	if cb == nil {
		return nil, MethodNotFound(req.Method)
	}
	if cb.hasAuth {
		r := ctx.HTTPRequest()
		if err := authRequire(r); err != nil {
			return nil, err
		}
	}
	if cb.notSingle && isBatch {
		return nil, InvalidInput("not support batch")
	}
	args, err := req.Params.UnmarshalValue(cb.argTypes)
	if err != nil {
		return nil, InvalidParamsError(err.Error())
	}

	return runMethod(ctx, s.mode, args, cb)
}

func runMethod(ctx RequestContext, m Mode, args []reflect.Value, cb *callback) (any, *Error) {
	return cb.call(ctx, m, args)
}

func (s *serviceRegistry) runBatch(reqCtx RequestContext, reqs []*Request) {
	s.callProc(func(ctx context.Context) {
		var (
			timer      *time.Timer
			cancel     context.CancelFunc
			callBuffer = &batchCallBuffer{reqs: reqs, resp: make([]*any, 0, len(reqs))}
		)
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		if timeout, ok := ContextRequestTimeout(ctx); ok {
			timer = time.AfterFunc(timeout, func() {
				cancel()
				callBuffer.write(reqCtx, true)
			})
		}
		for {
			// No need to handle rest of reqs if timed out.
			if ctx.Err() != nil {
				break
			}
			msg := callBuffer.nextRequest()
			if msg == nil {
				break
			}
			req := []*Request{msg}
			resp, err := s.run(reqCtx, true, req)
			callBuffer.pushResponse(&resp, err, msg)
		}
		if timer != nil {
			timer.Stop()
		}
		callBuffer.write(reqCtx, false)
	})

}

func (s *serviceRegistry) callProc(fn func(ctx context.Context)) {
	s.runWg.Add(1)
	go func() {
		ctx, cancel := context.WithCancel(s.ctx)
		defer s.runWg.Done()
		defer cancel()
		fn(ctx)
	}()
}

type batchCallBuffer struct {
	mutex sync.Mutex
	reqs  []*Request
	resp  []*any
	wrote bool
}

// nextRequest returns the next unprocessed request.
func (b *batchCallBuffer) nextRequest() *Request {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if len(b.reqs) == 0 {
		return nil
	}
	req := b.reqs[0]
	return req
}

// pushResponse add the response.
func (b *batchCallBuffer) pushResponse(answer *any, err *Error, req *Request) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	result := generateResponse(req, answer, err)
	b.resp = append(b.resp, &result)
	b.reqs = b.reqs[1:]
}

// write response.
func (b *batchCallBuffer) write(ctx RequestContext, isTimeOut bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.wrote {
		return
	}
	if isTimeOut {
		for _, req := range b.reqs {
			result := generateResponse(req, nil, InternalError("request timed out"))
			b.resp = append(b.resp, &result)
		}
	}
	w := ctx.HTTPResponseWriter()
	b.wrote = true // can only write once
	if len(b.resp) > 0 {
		WriteBatchResponse(w, b.resp)
	}
}
