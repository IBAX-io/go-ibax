package jsonrpc

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

// RequestHandlerFunc is a helper for handling JSONRPC Requests over HTTP
// It can be used by microservices to handle JSONRPC methods.  For example:
//
// http.Handle("/", RequestHandlerFunc(func(ctx context.Context, r *Request) (interface{}, *Error) {
//		if r.Method != "eth_blockNumber" {
//			return nil, MethodNotSupported(fmt.Sprintf("unsupported method %s", r.Method))
//		}
//
//		return "0x123456", nil
//	}))
func RequestHandlerFunc(fn requestHandlerFunc) *requestHandlerFunc {
	return &fn
}

type RequestContext struct {
	context.Context
}

func (rc *RequestContext) HTTPRequest() *http.Request {
	return rc.Value(contextKeyHTTPRequest).(*http.Request)
}

func (rc *RequestContext) HTTPResponseWriter() http.ResponseWriter {
	return rc.Value(contextKeyHTTPResponseWriter).(http.ResponseWriter)
}

func (rc *RequestContext) RawJSON() json.RawMessage {
	return rc.Value(contextKeyRawJSON).(json.RawMessage)
}

type contextKey string

func (c contextKey) String() string {
	return "jsonrpc context key " + string(c)
}

var (
	contextKeyHTTPRequest        = contextKey("HTTP Request")
	contextKeyHTTPResponseWriter = contextKey("HTTP Response Writer")
	contextKeyRawJSON            = contextKey("Raw JSON")
)

type requestHandlerFunc func(ctx RequestContext, request *Request) (any, *Error)

func (h requestHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	buff, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "invalid content type, only application/json is supported", http.StatusUnsupportedMediaType)
		return
	}

	request := Request{}
	err = json.Unmarshal(buff, &request)
	if err != nil {
		// return a generic jsonrpc response
		WriteResponse(w, nil, nil, InvalidRequest(err.Error()))
		return
	}

	ctx := context.WithValue(r.Context(), contextKeyHTTPRequest, r)
	ctx = context.WithValue(ctx, contextKeyRawJSON, json.RawMessage(buff))
	ctx = context.WithValue(ctx, contextKeyHTTPResponseWriter, w)

	result, e := h(RequestContext{ctx}, &request)
	WriteResponse(w, &request, result, e)
}

func WriteResponse(w http.ResponseWriter, request *Request, result any, e *Error) {
	response := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      *ID    `json:"id,omitempty"`
		Result  any    `json:"result,omitempty"`
		Error   any    `json:"error,omitempty"`
	}{
		JSONRPC: "2.0",
	}

	if request != nil {
		response.ID = &request.ID
	}

	response.Result = result
	if e != nil {
		// NOTE: setting `response.Error = e` if e is nil causes `"error": null` to be included
		// in the Response.  See https://play.golang.org/p/Oe3MFR3wwAu
		response.Error = e
	}

	b, err := json.Marshal(&response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
