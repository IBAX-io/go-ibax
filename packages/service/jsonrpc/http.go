/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package jsonrpc

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxRequestContentLength = 1024 * 1024 * 5
	contentType             = "application/json"
)

func validateRequest(r *http.Request) (int, error) {
	if r.Method == http.MethodPut || r.Method == http.MethodDelete {
		return http.StatusMethodNotAllowed, errors.New("method not allowed")
	}
	if r.ContentLength > maxRequestContentLength {
		err := fmt.Errorf("content length too large (%d>%d)", r.ContentLength, maxRequestContentLength)
		return http.StatusRequestEntityTooLarge, err
	}
	// Allow OPTIONS (regardless of content-type)
	if r.Method == http.MethodOptions {
		return 0, nil
	}
	// Check content-type
	if mt, _, err := mime.ParseMediaType(r.Header.Get("content-type")); err == nil {
		if mt == contentType {
			return 0, nil
		}
	}
	// Invalid content-type
	err := fmt.Errorf("invalid content type, only %s is supported", contentType)
	return http.StatusUnsupportedMediaType, err
}

func NewMiddlewares(srv http.Handler, m Mode) http.Handler {
	handler := newGzipHandler(srv)
	handler = clientMiddleware(handler, m)
	handler = tokenMiddleware(handler)
	handler = nodeStateMiddleware(handler)
	//handler = statsdMiddleware(handler)
	handler = recoverMiddleware(handler)
	handler = loggerMiddleware(handler)
	return limiterMiddleware(handler)
}

type gzipResponseWriter struct {
	resp http.ResponseWriter

	gz            *gzip.Writer
	contentLength uint64 // total length of the uncompressed response
	written       uint64 // amount of written bytes from the uncompressed response
	hasLength     bool   // true if uncompressed response had Content-Length
	hasInit       bool   // true after init was called for the first time
}

var gzPool = sync.Pool{
	New: func() interface{} {
		w := gzip.NewWriter(io.Discard)
		return w
	},
}

// init runs just before response headers are written. Among other things, this function
// also decides whether compression will be applied at all.
func (w *gzipResponseWriter) init() {
	if w.hasInit {
		return
	}
	w.hasInit = true

	hdr := w.resp.Header()
	length := hdr.Get("content-length")
	if len(length) > 0 {
		if n, err := strconv.ParseUint(length, 10, 64); err != nil {
			w.hasLength = true
			w.contentLength = n
		}
	}

	// Setting Transfer-Encoding to "identity" explicitly disables compression.
	setIdentity := hdr.Get("transfer-encoding") == "identity"
	if !setIdentity {
		w.gz = gzPool.Get().(*gzip.Writer)
		w.gz.Reset(w.resp)
		hdr.Del("content-length")
		hdr.Set("content-encoding", "gzip")
	}
}

func (w *gzipResponseWriter) Header() http.Header {
	return w.resp.Header()
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	w.init()
	w.resp.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	w.init()

	if w.gz == nil {
		// Compression is disabled.
		return w.resp.Write(b)
	}

	n, err := w.gz.Write(b)
	w.written += uint64(n)
	if w.hasLength && w.written >= w.contentLength {
		// The HTTP handler has finished writing the entire uncompressed response. Close
		// the gzip stream to ensure the footer will be seen by the client in case the
		// response is flushed after this call to write.
		err = w.gz.Close()
	}
	return n, err
}

func (w *gzipResponseWriter) Flush() {
	if w.gz != nil {
		w.gz.Flush()
	}
	if f, ok := w.resp.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *gzipResponseWriter) close() {
	if w.gz == nil {
		return
	}
	w.gz.Close()
	gzPool.Put(w.gz)
	w.gz = nil
}

func newGzipHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		wrapper := &gzipResponseWriter{resp: w}
		defer wrapper.close()

		next.ServeHTTP(wrapper, r)
	})
}

func ContextRequestTimeout(ctx context.Context) (time.Duration, bool) {
	timeout := time.Duration(math.MaxInt64)
	hasTimeout := false
	setTimeout := func(d time.Duration) {
		if d < timeout {
			timeout = d
			hasTimeout = true
		}
	}

	if deadline, ok := ctx.Deadline(); ok {
		setTimeout(time.Until(deadline))
	}

	// If the context is an HTTP request context, use the server's WriteTimeout.
	httpSrv, ok := ctx.Value(http.ServerContextKey).(*http.Server)
	if ok && httpSrv.WriteTimeout > 0 {
		wt := httpSrv.WriteTimeout
		// When a write timeout is configured, we need to send the response message before
		// the HTTP server cuts connection. So our internal timeout must be earlier than
		// the server's true timeout.
		//
		// Note: Timeouts are sanitized to be a minimum of 1 second.
		// Also see issue: https://github.com/golang/go/issues/47229
		wt -= 100 * time.Millisecond
		setTimeout(wt)
	}

	return timeout, hasTimeout
}
