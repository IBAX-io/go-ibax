/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
	r.Body = http.MaxBytesReader(w, r.Body, h.n)
	h.h.ServeHTTP(w, r)
}

func NewMaxBodyReader(h http.Handler, n int64) http.Handler {
	return &MaxBodyReader{h, n}
}
